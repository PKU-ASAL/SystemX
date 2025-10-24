#!/usr/bin/env python3
"""
SysArmor Threat Detection Engine
威胁检测规则引擎
基于 Falco/Sysdig 规则引擎设计
"""

import os
import json
import logging
import re
import yaml
import uuid
from datetime import datetime
from typing import Dict, List, Optional, Any

logger = logging.getLogger(__name__)


class FalcoConditionEvaluator:
    """Falco 条件表达式评估器"""
    
    def __init__(self):
        self.operators = {
            'equals': self._op_equals,
            'not_equals': self._op_not_equals,
            'in': self._op_in,
            'not_in': self._op_not_in,
            'contains': self._op_contains,
            'not_contains': self._op_not_contains,
            'startswith': self._op_startswith,
            'endswith': self._op_endswith,
            'regex': self._op_regex,
            'gt': self._op_gt,
            'gte': self._op_gte,
            'lt': self._op_lt,
            'lte': self._op_lte
        }
    
    def evaluate_condition(self, condition: Dict, event_data: Dict) -> bool:
        """评估条件表达式"""
        try:
            if 'and' in condition:
                return all(self.evaluate_condition(sub_cond, event_data) for sub_cond in condition['and'])
            elif 'or' in condition:
                return any(self.evaluate_condition(sub_cond, event_data) for sub_cond in condition['or'])
            elif 'not' in condition:
                return not self.evaluate_condition(condition['not'], event_data)
            elif 'field' in condition:
                return self._evaluate_field_condition(condition, event_data)
            else:
                logger.warning(f"未知条件类型: {condition}")
                return False
        except Exception as e:
            logger.debug(f"条件评估异常: {e}")
            return False
    
    def _evaluate_field_condition(self, condition: Dict, event_data: Dict) -> bool:
        """评估字段条件"""
        field_path = condition['field']
        operator = condition['operator']
        expected_value = condition.get('value', condition.get('values'))
        
        # 获取字段值
        actual_value = self._get_field_value(field_path, event_data)
        
        if actual_value is None:
            return False
        
        # 执行操作符比较
        op_func = self.operators.get(operator)
        if op_func:
            return op_func(actual_value, expected_value)
        else:
            logger.warning(f"未知操作符: {operator}")
            return False
    
    def _get_field_value(self, field_path: str, event_data: Dict):
        """根据字段路径获取值，支持嵌套访问和直接键名匹配"""
        try:
            # 首先尝试直接访问完整字段名（适配 sysdig 格式）
            if field_path in event_data:
                return event_data[field_path]
            
            # 如果直接访问失败，尝试分层访问
            parts = field_path.split('.')
            current = event_data
            
            for part in parts:
                if isinstance(current, dict):
                    # 处理数组索引，如 proc.aname[1]
                    if '[' in part and ']' in part:
                        field_name = part.split('[')[0]
                        index_str = part.split('[')[1].split(']')[0]
                        try:
                            index = int(index_str)
                            if field_name in current and isinstance(current[field_name], list):
                                if 0 <= index < len(current[field_name]):
                                    current = current[field_name][index]
                                else:
                                    return None
                            else:
                                return None
                        except (ValueError, IndexError):
                            return None
                    else:
                        current = current.get(part)
                else:
                    return None
                
                if current is None:
                    return None
            
            return current
        except Exception as e:
            logger.debug(f"获取字段值失败: {field_path}, {e}")
            return None
    
    # 操作符实现
    def _op_equals(self, actual, expected):
        return actual == expected
    
    def _op_not_equals(self, actual, expected):
        return actual != expected
    
    def _op_in(self, actual, expected):
        return actual in expected if isinstance(expected, (list, tuple)) else False
    
    def _op_not_in(self, actual, expected):
        return actual not in expected if isinstance(expected, (list, tuple)) else True
    
    def _op_contains(self, actual, expected):
        return str(expected) in str(actual) if actual is not None else False
    
    def _op_not_contains(self, actual, expected):
        return str(expected) not in str(actual) if actual is not None else True
    
    def _op_startswith(self, actual, expected):
        return str(actual).startswith(str(expected)) if actual is not None else False
    
    def _op_endswith(self, actual, expected):
        return str(actual).endswith(str(expected)) if actual is not None else False
    
    def _op_regex(self, actual, expected):
        try:
            return re.search(str(expected), str(actual), re.IGNORECASE) is not None if actual is not None else False
        except re.error:
            return False
    
    def _op_gt(self, actual, expected):
        try:
            return float(actual) > float(expected)
        except (ValueError, TypeError):
            return False
    
    def _op_gte(self, actual, expected):
        try:
            return float(actual) >= float(expected)
        except (ValueError, TypeError):
            return False
    
    def _op_lt(self, actual, expected):
        try:
            return float(actual) < float(expected)
        except (ValueError, TypeError):
            return False
    
    def _op_lte(self, actual, expected):
        try:
            return float(actual) <= float(expected)
        except (ValueError, TypeError):
            return False


class EventNormalizer:
    """事件数据标准化器 - 将不同格式的事件数据转换为统一的 Falco 字段格式"""
    
    @staticmethod
    def normalize_event_data(event: Dict[str, Any]) -> Dict[str, Any]:
        """将 sysdig 事件数据标准化为 Falco 字段格式，适配 SysArmor 数据结构"""
        message = event.get('message', {})
        
        # 构建标准化的事件数据结构，添加对现有数据结构的适配
        normalized = {
            # 事件基础信息 - 适配 SysArmor 数据结构
            'evt.type': message.get('evt.type', event.get('event_type', '')),
            'evt.time': message.get('evt.time', event.get('timestamp', '')),
            'evt.num': message.get('evt.num', 0),
            'evt.category': message.get('evt.category', event.get('event_category', '')),
            'evt.dir': message.get('evt.dir', '>'),
            'evt.args': message.get('evt.args', ''),
            
            # 模拟缺失的事件字段
            'evt.rawres': message.get('evt.res', 0),
            'evt.is_open_read': EventNormalizer._infer_open_read(message.get('evt.type', '')),
            'evt.is_open_write': EventNormalizer._infer_open_write(message.get('evt.type', '')),
            'evt.arg.request': EventNormalizer._extract_arg_request(message.get('evt.args', '')),
            'evt.arg.target': EventNormalizer._extract_arg_target(message.get('evt.args', '')),
            'evt.arg.oldpath': EventNormalizer._extract_arg_oldpath(message.get('evt.args', '')),
            'evt.arg.family': EventNormalizer._extract_arg_family(message.get('evt.args', '')),
            
            # 进程信息 - 直接映射现有字段
            'proc.name': message.get('proc.name', ''),
            'proc.exe': message.get('proc.exe', ''),
            'proc.exepath': message.get('proc.exe', ''),  # 使用 proc.exe 作为 exepath
            'proc.cmdline': message.get('proc.cmdline', ''),
            'proc.pcmdline': message.get('proc.pcmdline', ''),
            'proc.pid': message.get('proc.pid', 0),
            'proc.ppid': message.get('proc.ppid', 0),
            'proc.uid': message.get('proc.uid', 0),
            'proc.gid': message.get('proc.gid', 0),
            
            # 模拟缺失的进程字段
            'proc.pname': EventNormalizer._extract_parent_name(message.get('proc.pcmdline', '')),
            'proc.tty': EventNormalizer._extract_tty_from_args(message.get('evt.args', '')),
            'proc.pexe': '',  # 暂时为空
            'proc.pexepath': '',  # 暂时为空
            'proc.duration': 0,  # 暂时为0
            
            # 模拟祖先进程信息
            'proc.aname': EventNormalizer._build_ancestor_names(message),
            
            # 文件描述符信息 - 适配现有数据并提取缺失字段
            'fd.name': message.get('fd.name', ''),
            'fd.nameraw': message.get('fd.name', ''),  # 使用 fd.name 作为 nameraw
            'fd.type': EventNormalizer._infer_fd_type(message.get('net.sockaddr', {})),
            'fd.typechar': '',  # 暂时为空
            'fd.num': -1,  # 暂时为-1
            
            # 用户信息 - 从现有字段推导
            'user.name': EventNormalizer._get_user_name(message.get('proc.uid', 0)),
            'user.uid': message.get('proc.uid', 0),
            'user.loginuid': 0,  # 暂时为0
            
            # 网络信息 - 从现有数据中解析
            'net.sockaddr': message.get('net.sockaddr', {}),
            
            # 容器信息 - 默认为主机
            'container.id': 'host',  # 您的数据结构中似乎没有容器信息
            'container.privileged': False,  # 默认为非特权
            'container.image.repository': '',
            
            # 原始事件数据 (用于兼容)
            'message': message,
            'event': event
        }
        
        # 提取文件目录和文件名信息
        fd_name = normalized.get('fd.name', '')
        if fd_name:
            directory, filename = EventNormalizer._extract_file_info(fd_name)
            normalized['fd.directory'] = directory
            normalized['fd.filename'] = filename
        else:
            normalized['fd.directory'] = ''
            normalized['fd.filename'] = ''
        
        # 解析网络连接信息
        network_info = EventNormalizer._parse_network_info(
            message.get('net.sockaddr', {}), 
            fd_name
        )
        normalized.update(network_info)
        
        return normalized
    
    @staticmethod
    def _infer_open_read(evt_type: str) -> bool:
        """根据事件类型推断是否为读取操作"""
        read_types = ['open', 'openat', 'openat2', 'read', 'pread', 'readv', 'preadv']
        return evt_type in read_types
    
    @staticmethod
    def _infer_open_write(evt_type: str) -> bool:
        """根据事件类型推断是否为写入操作"""
        write_types = ['write', 'pwrite', 'writev', 'pwritev', 'truncate', 'ftruncate']
        return evt_type in write_types
    
    @staticmethod
    def _extract_arg_request(evt_args: str) -> str:
        """从 evt.args 中提取 ptrace 请求类型"""
        if 'PTRACE_ATTACH' in evt_args:
            return 'PTRACE_ATTACH'
        elif 'PTRACE_TRACEME' in evt_args:
            return 'PTRACE_TRACEME'
        elif 'PTRACE_POKETEXT' in evt_args:
            return 'PTRACE_POKETEXT'
        elif 'PTRACE_POKEDATA' in evt_args:
            return 'PTRACE_POKEDATA'
        return ''
    
    @staticmethod
    def _extract_arg_target(evt_args: str) -> str:
        """从 evt.args 中提取目标路径"""
        # 简单实现，可根据需要扩展
        return ''
    
    @staticmethod
    def _extract_arg_oldpath(evt_args: str) -> str:
        """从 evt.args 中提取旧路径"""
        # 简单实现，可根据需要扩展
        return ''
    
    @staticmethod
    def _extract_arg_family(evt_args: str) -> str:
        """从 evt.args 中提取地址族"""
        if 'AF_PACKET' in evt_args:
            return 'AF_PACKET'
        elif 'AF_UNIX' in evt_args:
            return 'AF_UNIX'
        elif 'AF_INET' in evt_args:
            return 'AF_INET'
        return ''
    
    @staticmethod
    def _extract_parent_name(pcmdline: str) -> str:
        """从父进程命令行中提取父进程名称"""
        if not pcmdline:
            return ''
        parts = pcmdline.split()
        if parts:
            return parts[0].split('/')[-1]  # 取路径的最后部分
        return ''
    
    @staticmethod
    def _extract_tty_from_args(evt_args: str) -> int:
        """从 evt.args 中提取 tty 信息"""
        match = re.search(r'tty=(\w+)', evt_args)
        if match and match.group(1) != 'pts0':
            return 1  # 非标准tty返回1
        return 0  # 标准tty或无tty返回0
    
    @staticmethod
    def _build_ancestor_names(message: dict) -> list:
        """构建祖先进程名称列表"""
        ancestors = []
        pcmdline = message.get('proc.pcmdline', '')
        if pcmdline:
            pname = EventNormalizer._extract_parent_name(pcmdline)
            if pname:
                ancestors.append(pname)
        return ancestors
    
    @staticmethod
    def _infer_fd_type(net_sockaddr: dict) -> str:
        """根据网络信息推断文件描述符类型"""
        if isinstance(net_sockaddr, dict):
            family = net_sockaddr.get('family', '')
            if family == 'AF_UNIX':
                return 'unix'
            elif family in ['AF_INET', 'AF_INET6']:
                socket_type = net_sockaddr.get('type', '')
                if 'tcp' in socket_type.lower():
                    return 'ipv4'
                elif 'udp' in socket_type.lower():
                    return 'ipv4'
        return 'file'
    
    @staticmethod
    def _get_user_name(uid: int) -> str:
        """根据 UID 获取用户名"""
        # 常见的系统用户映射
        system_users = {
            0: 'root',
            1: 'daemon', 
            2: 'bin',
            65534: 'nobody'
        }
        return system_users.get(uid, f'user_{uid}')
    
    @staticmethod
    def _extract_file_info(fd_name: str) -> tuple:
        """从文件描述符名称中提取目录和文件名"""
        if not fd_name or '->' in fd_name:
            return '', ''
        
        import os
        directory = os.path.dirname(fd_name)
        filename = os.path.basename(fd_name)
        return directory, filename
    
    @staticmethod
    def _parse_network_info(net_sockaddr: dict, fd_name: str) -> dict:
        """解析网络连接信息"""
        network_info = {
            'fd.sip': '',
            'fd.sport': 0,
            'fd.dip': '',
            'fd.dport': 0,
            'fd.sip.name': ''
        }
        
        # 处理 Unix socket
        if isinstance(net_sockaddr, dict):
            if net_sockaddr.get('family') == 'AF_UNIX':
                address = net_sockaddr.get('address', '')
                if address:
                    network_info['fd.sip.name'] = address
        
        # 从 fd.name 中解析网络信息 (格式: IP:port->dest)
        if '->' in fd_name:
            parts = fd_name.split('->')
            if len(parts) == 2:
                src = parts[0].strip()
                dst = parts[1].strip()
                
                # 解析源地址
                if ':' in src:
                    src_parts = src.rsplit(':', 1)
                    network_info['fd.sip'] = src_parts[0]
                    try:
                        network_info['fd.sport'] = int(src_parts[1])
                    except ValueError:
                        pass
                
                # 解析目标地址
                if ':' in dst and not dst.startswith('/'):
                    dst_parts = dst.rsplit(':', 1)
                    network_info['fd.dip'] = dst_parts[0]
                    try:
                        network_info['fd.dport'] = int(dst_parts[1])
                    except ValueError:
                        pass
                else:
                    # 可能是 Unix socket 路径或服务名
                    if not network_info['fd.sip.name']:
                        network_info['fd.sip.name'] = dst
        
        return network_info


class ThreatDetectionRules:
    """威胁检测规则引擎 - 基于 Falco 规则设计"""
    
    def __init__(self, rules_file: str = "/opt/flink/configs/rules/threat_detection_rules.yaml"):
        self.rules = {}
        self.rule_groups = {}
        self.global_settings = {}
        self.condition_evaluator = FalcoConditionEvaluator()
        self.event_normalizer = EventNormalizer()
        self.load_rules(rules_file)
        
    def load_rules(self, rules_file: str):
        """加载威胁检测规则"""
        try:
            if os.path.exists(rules_file):
                with open(rules_file, 'r', encoding='utf-8') as f:
                    config = yaml.safe_load(f)
                
                # 加载旧格式规则
                for rule in config.get('rules', []):
                    if rule.get('enabled', True):
                        self.rules[rule['id']] = rule
                
                # 加载新格式 Falco 条件规则
                for rule in config.get('falco_condition_rules', []):
                    if rule.get('enabled', True):
                        self.rules[rule['id']] = rule
                
                # 加载规则组
                self.rule_groups = config.get('rule_groups', {})
                
                # 加载全局设置
                self.global_settings = config.get('global_settings', {})
                
                logger.info(f"✅ 加载了 {len(self.rules)} 个威胁检测规则")
                logger.info(f"📋 规则组: {list(self.rule_groups.keys())}")
            else:
                logger.warning(f"规则文件不存在: {rules_file}，使用默认规则")
                self._load_default_rules()
                
        except Exception as e:
            logger.error(f"加载规则失败: {e}，使用默认规则")
            self._load_default_rules()
    
    def _load_default_rules(self):
        """加载默认规则 - Falco 样式条件"""
        self.rules = {
            "suspicious_tmp_execution": {
                "id": "suspicious_tmp_execution",
                "name": "可疑临时目录程序执行",
                "category": "suspicious_activity",
                "severity": "high",
                "base_score": 85,
                "condition": {
                    "and": [
                        {
                            "field": "evt.type",
                            "operator": "in",
                            "values": ["execve", "execveat"]
                        },
                        {
                            "or": [
                                {
                                    "field": "proc.exe", 
                                    "operator": "startswith",
                                    "value": "/tmp/"
                                },
                                {
                                    "field": "proc.exe",
                                    "operator": "startswith", 
                                    "value": "/dev/shm/"
                                },
                                {
                                    "field": "proc.exe",
                                    "operator": "startswith", 
                                    "value": "/var/tmp/"
                                }
                            ]
                        }
                    ]
                }
            },
            "sensitive_file_access": {
                "id": "sensitive_file_access",
                "name": "敏感文件访问检测",
                "category": "file_access",
                "severity": "medium",
                "base_score": 70,
                "condition": {
                    "and": [
                        {
                            "field": "evt.type",
                            "operator": "in",
                            "values": ["open", "openat", "openat2"]
                        },
                        {
                            "field": "fd.name",
                            "operator": "in",
                            "values": ["/etc/shadow", "/etc/passwd", "/etc/sudoers"]
                        }
                    ]
                }
            }
        }
        logger.info("✅ 加载了默认威胁检测规则")
    
    def evaluate_event(self, event: Dict[str, Any]) -> List[Dict[str, Any]]:
        """评估事件是否触发威胁检测规则"""
        alerts = []
        
        # 标准化事件数据结构，适配 Falco 字段格式
        normalized_event = self.event_normalizer.normalize_event_data(event)
        
        for rule_id, rule in self.rules.items():
            if self._match_rule(rule_id, normalized_event, rule):
                alert = self._create_alert(event, rule)
                alerts.append(alert)
        
        return alerts
    
    def _match_rule(self, rule_id: str, event_data: Dict, rule: Dict) -> bool:
        """检查事件是否匹配规则 - 使用 Falco 条件表达式"""
        try:
            # 优先使用新的 Falco 条件格式
            if 'condition' in rule:
                return self.condition_evaluator.evaluate_condition(rule['condition'], event_data)
            
            # 兼容旧的关键词和正则格式
            event_str = json.dumps(event_data, ensure_ascii=False)
            
            # 检查关键词匹配
            keywords = rule.get('keywords', [])
            for keyword in keywords:
                if keyword in event_str:
                    return True
            
            # 检查正则表达式匹配
            patterns = rule.get('patterns', [])
            for pattern in patterns:
                if re.search(pattern, event_str, re.IGNORECASE):
                    return True
            
            # 检查简单字段条件匹配
            conditions = rule.get('conditions', {})
            if conditions:
                for field, expected_value in conditions.items():
                    actual_value = self.condition_evaluator._get_field_value(field, event_data)
                    if actual_value == expected_value:
                        return True
            
            return False
            
        except Exception as e:
            logger.debug(f"规则匹配异常 {rule_id}: {e}")
            return False
    
    def _create_alert(self, event: Dict, rule: Dict) -> Dict[str, Any]:
        """创建告警事件"""
        now = datetime.utcnow()
        
        # 计算风险评分
        base_score = rule.get('base_score', 50)
        score_multiplier = rule.get('score_multiplier', 1.0)
        final_score = min(100, int(base_score * score_multiplier))
        
        # 确定严重程度
        severity = rule.get('severity', 'medium')
        if final_score >= 90:
            severity = 'critical'
        elif final_score >= 70:
            severity = 'high'
        elif final_score >= 50:
            severity = 'medium'
        else:
            severity = 'low'
        
        alert = {
            # OpenSearch 标准主时间字段
            "@timestamp": now.isoformat() + 'Z',
            
            # 告警核心信息
            "alert": {
                "id": str(uuid.uuid4()),
                "type": "rule_based_detection",
                "category": rule.get('category', 'unknown'),
                "severity": severity,
                "risk_score": final_score,
                "confidence": 0.8,
                "rule": {
                    "id": rule['id'],
                    "name": rule.get('name', ''),
                    "description": rule.get('description', ''),
                    "title": f"{rule.get('name', 'Unknown Threat')}: {event.get('event_type', 'unknown')}",
                    "mitigation": f"检查 {rule.get('category', 'unknown')} 相关活动",
                    "references": [f"SysArmor Rule: {rule['id']}"]
                },
                "evidence": {
                    "event_type": event.get('event_type', ''),
                    "process_name": event.get('message', {}).get('proc.name', ''),
                    "process_cmdline": event.get('message', {}).get('proc.cmdline', ''),
                    "file_path": event.get('message', {}).get('fd.name', ''),
                    "network_info": event.get('message', {}).get('net.sockaddr', {})
                }
            },
            
            # 原始事件数据
            "event": {
                "raw": {
                    "event_id": event.get('event_id', ''),
                    "timestamp": event.get('timestamp', ''),
                    "source": event.get('source', 'auditd'),
                    "message": event.get('message', {})  # 完整的 sysdig 数据，包含 evt.time
                }
            },
            
            # 时间信息
            "timing": {
                "created_at": now.isoformat() + 'Z',
                "processed_at": now.isoformat() + 'Z'
            },
            
            # 元数据信息
            "metadata": {
                "collector_id": event.get('collector_id', ''),
                "host": event.get('host', 'unknown'),
                "source": "sysarmor-threat-detector",
                "processor": "flink-events-to-alerts"
            }
        }
        
        return alert
