#!/usr/bin/env python3
"""
SysArmor Processor - Events to Alerts Job
消费 sysarmor.events.audit topic，基于威胁检测规则过滤出告警事件
输出到 sysarmor.alerts 和 sysarmor.alerts.high topics
基于 Falco/Sysdig 规则引擎设计
"""

import os
import json
import logging
import re
import yaml
import requests
import uuid
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Any
from collections import defaultdict, deque
from pyflink.datastream import StreamExecutionEnvironment, CheckpointingMode
from pyflink.datastream.connectors.kafka import FlinkKafkaConsumer, FlinkKafkaProducer
# 移除不兼容的 ElasticsearchSink 导入
from pyflink.common.serialization import SimpleStringSchema
from pyflink.common.typeinfo import Types
from pyflink.datastream.functions import MapFunction, FilterFunction
from pyflink.datastream.state import ValueStateDescriptor
from pyflink.datastream.functions import KeyedProcessFunction, ProcessFunction
from pyflink.common import Time

# 配置日志
logging.basicConfig(level=logging.INFO)
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

class ThreatDetectionRules:
    """威胁检测规则引擎 - 基于 Falco 规则设计"""
    
    def __init__(self, rules_file: str = "/opt/flink/configs/rules/threat_detection_rules.yaml"):
        self.rules = {}
        self.rule_groups = {}
        self.global_settings = {}
        self.condition_evaluator = FalcoConditionEvaluator()
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
        normalized_event = self._normalize_event_data(event)
        
        for rule_id, rule in self.rules.items():
            if self._match_rule(rule_id, normalized_event, rule):
                alert = self._create_alert(event, rule)
                alerts.append(alert)
        
        return alerts
    
    def _normalize_event_data(self, event: Dict[str, Any]) -> Dict[str, Any]:
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
            'evt.is_open_read': self._infer_open_read(message.get('evt.type', '')),
            'evt.is_open_write': self._infer_open_write(message.get('evt.type', '')),
            'evt.arg.request': self._extract_arg_request(message.get('evt.args', '')),
            'evt.arg.target': self._extract_arg_target(message.get('evt.args', '')),
            'evt.arg.oldpath': self._extract_arg_oldpath(message.get('evt.args', '')),
            'evt.arg.family': self._extract_arg_family(message.get('evt.args', '')),
            
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
            'proc.pname': self._extract_parent_name(message.get('proc.pcmdline', '')),
            'proc.tty': self._extract_tty_from_args(message.get('evt.args', '')),
            'proc.pexe': '',  # 暂时为空
            'proc.pexepath': '',  # 暂时为空
            'proc.duration': 0,  # 暂时为0
            
            # 模拟祖先进程信息
            'proc.aname': self._build_ancestor_names(message),
            
            # 文件描述符信息 - 适配现有数据并提取缺失字段
            'fd.name': message.get('fd.name', ''),
            'fd.nameraw': message.get('fd.name', ''),  # 使用 fd.name 作为 nameraw
            'fd.type': self._infer_fd_type(message.get('net.sockaddr', {})),
            'fd.typechar': '',  # 暂时为空
            'fd.num': -1,  # 暂时为-1
            
            # 用户信息 - 从现有字段推导
            'user.name': self._get_user_name(message.get('proc.uid', 0)),
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
            directory, filename = self._extract_file_info(fd_name)
            normalized['fd.directory'] = directory
            normalized['fd.filename'] = filename
        else:
            normalized['fd.directory'] = ''
            normalized['fd.filename'] = ''
        
        # 解析网络连接信息
        network_info = self._parse_network_info(
            message.get('net.sockaddr', {}), 
            fd_name
        )
        normalized.update(network_info)
        
        return normalized
    
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
    
    def _infer_open_read(self, evt_type: str) -> bool:
        """根据事件类型推断是否为读取操作"""
        read_types = ['open', 'openat', 'openat2', 'read', 'pread', 'readv', 'preadv']
        return evt_type in read_types
    
    def _infer_open_write(self, evt_type: str) -> bool:
        """根据事件类型推断是否为写入操作"""
        write_types = ['write', 'pwrite', 'writev', 'pwritev', 'truncate', 'ftruncate']
        return evt_type in write_types
    
    def _extract_arg_request(self, evt_args: str) -> str:
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
    
    def _extract_arg_target(self, evt_args: str) -> str:
        """从 evt.args 中提取目标路径"""
        # 简单实现，可根据需要扩展
        return ''
    
    def _extract_arg_oldpath(self, evt_args: str) -> str:
        """从 evt.args 中提取旧路径"""
        # 简单实现，可根据需要扩展
        return ''
    
    def _extract_arg_family(self, evt_args: str) -> str:
        """从 evt.args 中提取地址族"""
        if 'AF_PACKET' in evt_args:
            return 'AF_PACKET'
        elif 'AF_UNIX' in evt_args:
            return 'AF_UNIX'
        elif 'AF_INET' in evt_args:
            return 'AF_INET'
        return ''
    
    def _extract_parent_name(self, pcmdline: str) -> str:
        """从父进程命令行中提取父进程名称"""
        if not pcmdline:
            return ''
        parts = pcmdline.split()
        if parts:
            return parts[0].split('/')[-1]  # 取路径的最后部分
        return ''
    
    def _extract_tty_from_args(self, evt_args: str) -> int:
        """从 evt.args 中提取 tty 信息"""
        import re
        match = re.search(r'tty=(\w+)', evt_args)
        if match and match.group(1) != 'pts0':
            return 1  # 非标准tty返回1
        return 0  # 标准tty或无tty返回0
    
    def _build_ancestor_names(self, message: dict) -> list:
        """构建祖先进程名称列表"""
        ancestors = []
        pcmdline = message.get('proc.pcmdline', '')
        if pcmdline:
            pname = self._extract_parent_name(pcmdline)
            if pname:
                ancestors.append(pname)
        return ancestors
    
    def _infer_fd_type(self, net_sockaddr: dict) -> str:
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
    
    def _get_user_name(self, uid: int) -> str:
        """根据 UID 获取用户名"""
        # 常见的系统用户映射
        system_users = {
            0: 'root',
            1: 'daemon', 
            2: 'bin',
            65534: 'nobody'
        }
        return system_users.get(uid, f'user_{uid}')
    
    def _extract_file_info(self, fd_name: str) -> tuple:
        """从文件描述符名称中提取目录和文件名"""
        if not fd_name or '->' in fd_name:
            return '', ''
        
        import os
        directory = os.path.dirname(fd_name)
        filename = os.path.basename(fd_name)
        return directory, filename
    
    def _parse_network_info(self, net_sockaddr: dict, fd_name: str) -> dict:
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

class EventToAlertsProcessor(MapFunction):
    """事件到告警处理器 - 简化版本，先测试基本功能"""
    
    def __init__(self):
        self.rules_engine = ThreatDetectionRules()
        
    def map(self, value):
        try:
            event = json.loads(value)
            logger.info(f"🔍 处理事件: {event.get('event_type', 'unknown')} from {event.get('collector_id', 'unknown')[:8]}")
            
            # 基础规则匹配
            alerts = self.rules_engine.evaluate_event(event)
            
            if alerts:
                logger.info(f"🚨 匹配到 {len(alerts)} 个告警规则")
                # 返回第一个匹配的告警
                alert = alerts[0]
                logger.info(f"🚨 生成告警: {alert['alert']['id']} - {alert['alert']['rule']['name']}")
                return json.dumps(alert, ensure_ascii=False)
            
            return None
                
        except Exception as e:
            logger.error(f"处理事件异常: {e}")
            return None

class AlertSeverityRouter(FilterFunction):
    """告警严重程度路由器"""
    
    def __init__(self, target_severity: str = "high"):
        self.target_severity = target_severity
    
    def filter(self, value):
        try:
            alert = json.loads(value)
            severity = alert.get('alert', {}).get('severity', 'low')
            
            if self.target_severity == "high":
                return severity in ['high', 'critical']
            else:
                return severity in ['low', 'medium']
                
        except Exception:
            return False

def main():
    """主函数：创建事件到告警的处理作业"""
    
    logger.info("🚀 Starting SysArmor Audit Events to Alerts Job")
    logger.info("📋 Based on Falco-style rule engine")
    logger.info("📊 Processing: sysarmor.events.audit → sysarmor.alerts.audit")
    
    # 环境变量配置
    kafka_servers = os.getenv('KAFKA_BOOTSTRAP_SERVERS', 'middleware-kafka:9092')
    input_topic = 'sysarmor.events.audit'
    output_topic = 'sysarmor.alerts.audit'  # 简化为单一告警topic
    kafka_group_id = 'sysarmor-audit-events-to-alerts-processor'  # 更新Consumer Group名称
    
    logger.info(f"📡 Kafka Servers: {kafka_servers}")
    logger.info(f"📥 Input Topic: {input_topic}")
    logger.info(f"📤 Output Topic: {output_topic}")
    logger.info(f"👥 Consumer Group: {kafka_group_id}")
    
    # 创建流处理环境
    env = StreamExecutionEnvironment.get_execution_environment()
    
    # 配置环境
    env.set_parallelism(2)  # 2个并行度
    env.enable_checkpointing(30000)  # 30秒 checkpoint
    env.get_checkpoint_config().set_checkpointing_mode(CheckpointingMode.EXACTLY_ONCE)
    
    try:
        # 添加 JAR 依赖
        env.add_jars("file:///opt/flink/lib/flink-sql-connector-kafka-3.1.0-1.18.jar")
        
        # 创建 Kafka Consumer
        consumer_props = {
            'bootstrap.servers': kafka_servers,
            'group.id': kafka_group_id,
            'auto.offset.reset': 'earliest',  # 处理所有事件，包括历史事件
            'session.timeout.ms': '30000',
            'heartbeat.interval.ms': '10000',
            'max.poll.interval.ms': '300000'
        }
        
        kafka_consumer = FlinkKafkaConsumer(
            topics=[input_topic],
            deserialization_schema=SimpleStringSchema(),
            properties=consumer_props
        )
        
        # 创建 Kafka Producer (简化为单一告警流)
        producer_props = {
            'bootstrap.servers': kafka_servers,
            'transaction.timeout.ms': '900000',
            'batch.size': '16384',
            'linger.ms': '5',
            'compression.type': 'snappy'
        }
        
        kafka_producer = FlinkKafkaProducer(
            topic=output_topic,
            serialization_schema=SimpleStringSchema(),
            producer_config=producer_props
        )
        
        logger.info("📋 Creating Falco-style threat detection pipeline...")
        
        # 构建数据流处理管道
        events_stream = env.add_source(kafka_consumer)
        
        # 按 collector_id 分组，支持频率检测
        keyed_stream = events_stream.key_by(
            lambda event: json.loads(event).get('collector_id', 'unknown')
        )
        
        # 威胁检测处理 (简化版本)
        alerts_stream = events_stream.map(
            EventToAlertsProcessor(),
            output_type=Types.STRING()
        ).filter(lambda x: x is not None)
        
        # 配置 OpenSearch sink
        opensearch_url = os.getenv('OPENSEARCH_URL', 'http://opensearch:9200')
        opensearch_username = os.getenv('OPENSEARCH_USERNAME', 'admin')
        opensearch_password = os.getenv('OPENSEARCH_PASSWORD', 'admin')
        
        logger.info(f"🔍 OpenSearch URL: {opensearch_url}")
        
        # 创建 OpenSearch HTTP Sink (借鉴工作版本的方案)
        class OpenSearchHttpSink(MapFunction):
            """OpenSearch HTTP Sink - 使用 HTTP 请求写入"""
            
            def __init__(self):
                self.opensearch_url = opensearch_url
                self.opensearch_username = opensearch_username
                self.opensearch_password = opensearch_password
                self.index_url = f"{opensearch_url}/sysarmor-alerts-audit/_doc"
                
            def map(self, value):
                try:
                    if not value:
                        return value
                    
                    alert_data = json.loads(value)
                    
                    headers = {'Content-Type': 'application/json'}
                    auth = (self.opensearch_username, self.opensearch_password)
                    
                    response = requests.post(
                        self.index_url,
                        json=alert_data,
                        headers=headers,
                        auth=auth,
                        timeout=10,
                        verify=False
                    )
                    
                    if response.status_code in [200, 201]:
                        logger.info(f"✅ 告警写入 OpenSearch: {alert_data.get('alert', {}).get('id', 'unknown')}")
                    else:
                        logger.error(f"❌ OpenSearch 写入失败: {response.status_code}")
                        
                except Exception as e:
                    logger.error(f"❌ OpenSearch HTTP Sink 错误: {e}")
                
                return value
        
        opensearch_http_sink = OpenSearchHttpSink()
        logger.info("✅ OpenSearch HTTP sink 已配置: sysarmor-alerts-audit")
        
        # 简化的告警输出 (单一告警流)
        alerts_stream.add_sink(kafka_producer)
        
        # 所有告警写入 OpenSearch (使用 HTTP 方式)
        alerts_stream.map(opensearch_http_sink, output_type=Types.STRING())
        logger.info("✅ 所有告警将写入 OpenSearch: sysarmor-alerts-audit")
        
        logger.info("✅ 告警将写入 Kafka Topic + OpenSearch")
        
        # 监控输出
        alerts_stream.map(
            lambda x: f"🚨 Alert: {json.loads(x).get('alert', {}).get('severity', 'unknown')} - {json.loads(x).get('alert', {}).get('rule', {}).get('name', 'unknown')} from {json.loads(x).get('metadata', {}).get('collector_id', 'unknown')[:8]}",
            output_type=Types.STRING()
        ).print()
        
        logger.info("🔄 Falco-style threat detection pipeline created:")
        logger.info(f"   {input_topic} -> Rule Engine -> Threat Detection -> {output_topic}")
        
        # 显示加载的规则
        rules_engine = ThreatDetectionRules()
        logger.info("🛡️ 加载的威胁检测规则:")
        for rule_id, rule in rules_engine.rules.items():
            logger.info(f"   - {rule_id}: {rule.get('name', '')} ({rule.get('severity', 'unknown')})")
        
        logger.info("🎯 告警输出:")
        logger.info(f"   - Kafka Topic: {output_topic}")
        logger.info(f"   - OpenSearch索引: sysarmor-alerts-audit")
        
        # 执行作业
        logger.info("✅ Starting audit threat detection job...")
        
        job_client = env.execute_async("SysArmor-Audit-Events-to-Alerts-Processor")
        
        logger.info(f"🎯 Audit Events to Alerts job submitted successfully!")
        logger.info(f"📋 Job submitted with async execution")
        logger.info(f"🌐 Monitor at: http://localhost:8081")
        logger.info(f"📊 Processing: {input_topic} → {output_topic}")
        logger.info(f"🔍 View logs: docker logs -f sysarmor-flink-taskmanager-1")
        
        return "async-job-submitted"
        
    except Exception as e:
        logger.error(f"❌ Events to Alerts job failed: {e}")
        raise

if __name__ == "__main__":
    main()
