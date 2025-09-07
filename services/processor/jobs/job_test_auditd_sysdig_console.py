#!/usr/bin/env python3
"""
SysArmor Processor - Auditd to Sysdig Console Test Job
消费 sysarmor-events-test topic，将 auditd 数据转换为 sysdig 格式并输出到控制台
基于 NODLINK 管道的处理逻辑实现
"""

import os
import json
import logging
import re
from datetime import datetime
from typing import Dict, List, Optional, Any
from pyflink.datastream import StreamExecutionEnvironment, CheckpointingMode
from pyflink.datastream.connectors.kafka import FlinkKafkaConsumer
from pyflink.common.serialization import SimpleStringSchema
from pyflink.common.typeinfo import Types
from pyflink.datastream.functions import MapFunction, FilterFunction

# 配置日志
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class AuditdSysdigParser:
    """基于 NODLINK 管道逻辑的 Auditd 到 Sysdig 转换器"""
    
    # 系统调用号到事件类型的映射 (基于 NODLINK 支持的事件类型)
    SYSCALL_MAP = {
        0: "read", 1: "write", 2: "open", 3: "close", 4: "stat", 5: "fstat",
        6: "lstat", 7: "poll", 8: "lseek", 9: "mmap", 10: "mprotect", 11: "munmap",
        12: "brk", 13: "rt_sigaction", 14: "rt_sigprocmask", 15: "rt_sigreturn",
        16: "ioctl", 17: "pread64", 18: "pwrite64", 19: "readv", 20: "writev",
        21: "access", 22: "pipe", 23: "select", 24: "sched_yield", 25: "mremap",
        26: "msync", 27: "mincore", 28: "madvise", 29: "shmget", 30: "shmat",
        31: "shmctl", 32: "dup", 33: "dup2", 34: "pause", 35: "nanosleep",
        36: "getitimer", 37: "alarm", 38: "setitimer", 39: "getpid", 40: "sendfile",
        41: "socket", 42: "connect", 43: "accept", 44: "sendto", 45: "recvfrom",
        46: "sendmsg", 47: "recvmsg", 48: "shutdown", 49: "bind", 50: "listen",
        51: "getsockname", 52: "getpeername", 53: "socketpair", 54: "setsockopt",
        55: "getsockopt", 56: "clone", 57: "fork", 58: "vfork", 59: "execve",
        60: "exit", 61: "wait4", 62: "kill", 63: "uname", 64: "semget",
        65: "semop", 66: "semctl", 67: "shmdt", 68: "msgget", 69: "msgsnd",
        70: "msgrcv", 71: "msgctl", 72: "fcntl", 73: "flock", 74: "fsync",
        75: "fdatasync", 76: "truncate", 77: "ftruncate", 78: "getdents",
        79: "getcwd", 80: "chdir", 81: "fchdir", 82: "rename", 83: "mkdir",
        84: "rmdir", 85: "creat", 86: "link", 87: "unlink", 88: "symlink",
        89: "readlink", 90: "chmod", 91: "fchmod", 92: "chown", 93: "fchown",
        94: "lchown", 95: "umask", 96: "gettimeofday", 97: "getrlimit",
        98: "getrusage", 99: "sysinfo", 100: "times", 257: "openat"
    }
    
    # NODLINK 标准支持的事件类型 (基于 NODLINK 管道文档)
    NODLINK_SUPPORTED_EVENTS = {
        "read", "readv", "write", "writev", "fcntl", "rmdir", "rename", "chmod",
        "execve", "clone", "pipe", "fork", "accept", "sendmsg", "recvmsg", 
        "recvfrom", "send", "sendto", "open", "openat", "socket", "connect"
    }
    
    def __init__(self):
        self.process_cache = {}  # 进程缓存，用于进程树重建
        self.message_count = 0
        
    def parse_auditd_message(self, message: str) -> Optional[Dict[str, Any]]:
        """解析 auditd 消息，提取关键信息"""
        try:
            # 匹配 auditd 格式: type=SYSCALL msg=audit(timestamp:id): fields...
            match = re.match(r'type=([^ ]+) msg=audit\(([\d.]+):(\d+)\): (.*)', message)
            if not match:
                return None
                
            record_type, timestamp, event_id, fields_str = match.groups()
            
            # 解析字段
            fields = {}
            field_pattern = r'(\w+)=("[^"]*"|\S+)'
            for field_match in re.finditer(field_pattern, fields_str):
                key, value = field_match.groups()
                # 去除引号
                if value.startswith('"') and value.endswith('"'):
                    value = value[1:-1]
                fields[key] = value
            
            return {
                'type': record_type,
                'timestamp': float(timestamp),
                'event_id': event_id,
                'fields': fields
            }
        except Exception as e:
            logger.debug(f"Failed to parse auditd message: {message[:100]}..., error: {e}")
            return None
    
    def decode_hex_cmdline(self, hex_str: str) -> str:
        """解码十六进制编码的命令行 (基于 NODLINK 管道逻辑)"""
        if not hex_str:
            return ""
        
        # 检查是否为十六进制字符串
        if not all(c in '0123456789abcdefABCDEF' for c in hex_str):
            return hex_str
        
        if len(hex_str) % 2 != 0:
            return hex_str
        
        try:
            hex_bytes = bytes.fromhex(hex_str)
            # 按 null 字节分割，重建命令行参数
            parts = hex_bytes.split(b'\x00')
            return ' '.join(part.decode('utf-8', errors='replace') for part in parts if part).strip()
        except:
            return hex_str
    
    def convert_to_sysdig_format(self, audit_data: Dict[str, Any], host: str) -> Optional[Dict[str, Any]]:
        """将 auditd 数据转换为 sysdig 格式 (基于 NODLINK 标准)"""
        try:
            if audit_data['type'] != 'SYSCALL':
                return None
            
            fields = audit_data['fields']
            
            # 获取系统调用类型
            syscall_num = int(fields.get('syscall', '0'))
            evt_type = self.SYSCALL_MAP.get(syscall_num, f"syscall_{syscall_num}")
            
            # 只处理 NODLINK 支持的事件类型
            if evt_type not in self.NODLINK_SUPPORTED_EVENTS:
                return None
            
            # 构建 sysdig 格式事件 (符合 NODLINK 标准)
            sysdig_event = {
                "evt.num": int(audit_data['event_id']),
                "evt.time": audit_data['timestamp'],
                "evt.type": evt_type,
                "evt.category": self._get_event_category(evt_type),
                "proc.name": fields.get('comm', '').strip('"'),
                "proc.exe": fields.get('exe', '').strip('"'),
                "proc.pid": int(fields.get('pid', '0')),
                "proc.ppid": int(fields.get('ppid', '0')),
                "proc.uid": int(fields.get('uid', '0')),
                "proc.gid": int(fields.get('gid', '0')),
                "host": host,
                "is_warn": False
            }
            
            # 处理命令行 (支持十六进制解码)
            if 'proctitle' in fields:
                cmdline = self.decode_hex_cmdline(fields['proctitle'])
                sysdig_event["proc.cmdline"] = cmdline if cmdline else sysdig_event["proc.name"]
            else:
                sysdig_event["proc.cmdline"] = sysdig_event["proc.name"]
            
            # 处理文件路径 (对于文件操作)
            if evt_type in ['open', 'openat', 'read', 'write', 'chmod', 'rename']:
                if 'name' in fields:
                    fd_name = fields['name'].strip('"')
                    if fd_name and fd_name != '(null)':
                        sysdig_event["fd.name"] = fd_name
            
            # 处理网络事件
            if evt_type in ['socket', 'connect', 'accept', 'sendto', 'recvfrom', 'sendmsg', 'recvmsg']:
                self._add_network_fields(sysdig_event, fields)
            
            # 更新进程缓存 (用于进程树重建)
            self._update_process_cache(sysdig_event)
            
            # 添加父进程命令行 (基于缓存重建)
            parent_cmdline = self._get_parent_cmdline(
                sysdig_event["proc.ppid"], 
                sysdig_event["evt.time"]
            )
            sysdig_event["proc.pcmdline"] = parent_cmdline
            
            return sysdig_event
            
        except Exception as e:
            logger.error(f"Failed to convert audit data to sysdig: {e}")
            return None
    
    def _get_event_category(self, evt_type: str) -> str:
        """获取事件类别 (基于 NODLINK 分类)"""
        file_events = {"read", "readv", "write", "writev", "open", "openat", "fcntl", "chmod", "rename", "rmdir"}
        process_events = {"execve", "clone", "fork", "exit"}
        network_events = {"socket", "connect", "accept", "sendto", "recvfrom", "sendmsg", "recvmsg"}
        ipc_events = {"pipe"}
        
        if evt_type in file_events:
            return "file"
        elif evt_type in process_events:
            return "process"
        elif evt_type in network_events:
            return "net"
        elif evt_type in ipc_events:
            return "ipc"
        else:
            return "other"
    
    def _add_network_fields(self, event: Dict[str, Any], fields: Dict[str, str]):
        """添加网络相关字段"""
        # 网络地址和端口信息
        if 'saddr' in fields:
            event["fd.sip"] = fields['saddr']
        if 'daddr' in fields:
            event["fd.dip"] = fields['daddr']
        if 'sport' in fields:
            try:
                event["fd.sport"] = int(fields['sport'])
            except:
                pass
        if 'dport' in fields:
            try:
                event["fd.dport"] = int(fields['dport'])
            except:
                pass
    
    def _update_process_cache(self, event: Dict[str, Any]):
        """更新进程缓存 (用于进程树重建)"""
        pid = event["proc.pid"]
        self.process_cache[pid] = {
            'cmdline': event["proc.cmdline"],
            'timestamp': event["evt.time"],
            'name': event["proc.name"]
        }
        
        # 限制缓存大小，避免内存泄漏
        if len(self.process_cache) > 10000:
            # 删除最旧的 1000 个条目
            oldest_pids = sorted(self.process_cache.keys(), 
                               key=lambda pid: self.process_cache[pid]['timestamp'])[:1000]
            for pid in oldest_pids:
                del self.process_cache[pid]
    
    def _get_parent_cmdline(self, ppid: int, event_time: float) -> str:
        """获取父进程命令行 (基于缓存和系统进程映射)"""
        if not ppid:
            return ""
        
        # 系统进程映射 (基于 NODLINK 管道逻辑)
        system_processes = {
            1: 'systemd --system --deserialize',
            2: '[kthreadd]',
            0: ''
        }
        
        if ppid in system_processes:
            return system_processes[ppid]
        
        # 从缓存中查找 (时间窗口 ±60秒)
        if ppid in self.process_cache:
            cached_info = self.process_cache[ppid]
            if abs(cached_info['timestamp'] - event_time) <= 60:
                return cached_info['cmdline']
        
        return ""  # 无法重建父进程信息


class AuditdToSysdigConsoleConverter(MapFunction):
    """Auditd 到 Sysdig 格式转换器 (控制台输出)"""
    
    def __init__(self):
        self.parser = AuditdSysdigParser()
        self.message_count = 0
        
    def map(self, value):
        try:
            if not value:
                return None
                
            # 解析输入的 JSON 消息
            data = json.loads(value)
            message = data.get('message', '')
            host = data.get('host', 'unknown')
            collector_id = data.get('collector_id', 'unknown')
            timestamp = data.get('timestamp', '')
            
            # 解析 auditd 消息
            audit_data = self.parser.parse_auditd_message(message)
            if not audit_data:
                return None
            
            # 转换为 sysdig 格式
            sysdig_event = self.parser.convert_to_sysdig_format(audit_data, host)
            if not sysdig_event:
                return None
            
            self.message_count += 1
            
            # 同时输出格式化的控制台显示和完整的 JSON 数据
            console_output = self._format_console_output(
                sysdig_event, host, collector_id[:8], timestamp, self.message_count
            )
            
            # 输出完整的 sysdig JSON 数据结构
            json_output = json.dumps(sysdig_event, ensure_ascii=False, indent=2)
            
            # 组合输出：控制台格式 + JSON 数据
            combined_output = f"{console_output}\n📊 SYSDIG JSON #{self.message_count}:\n{json_output}\n" + "="*80
            
            return combined_output
            
        except Exception as e:
            logger.error(f"Error in AuditdToSysdigConsoleConverter: {e}")
            return None
    
    def _format_console_output(self, sysdig_event: Dict[str, Any], host: str, 
                             collector_id: str, timestamp: str, count: int) -> str:
        """格式化控制台输出"""
        try:
            # 提取关键信息
            evt_type = sysdig_event.get("evt.type", "unknown")
            evt_category = sysdig_event.get("evt.category", "other")
            proc_name = sysdig_event.get("proc.name", "unknown")
            proc_pid = sysdig_event.get("proc.pid", 0)
            proc_cmdline = sysdig_event.get("proc.cmdline", "")
            fd_name = sysdig_event.get("fd.name", "")
            
            # 构建输出行
            time_str = timestamp[:19] if timestamp else "unknown"
            
            # 基本信息
            output_parts = [
                f"🔄 SYSDIG #{count}",
                f"{time_str}",
                f"{host}",
                f"{collector_id}",
                f"[{evt_category.upper()}]",
                f"{evt_type}",
                f"pid={proc_pid}",
                f"proc={proc_name}"
            ]
            
            # 添加命令行 (截断显示)
            if proc_cmdline and proc_cmdline != proc_name:
                cmdline_short = proc_cmdline[:50] + "..." if len(proc_cmdline) > 50 else proc_cmdline
                output_parts.append(f"cmd='{cmdline_short}'")
            
            # 添加文件路径
            if fd_name:
                fd_short = fd_name[:30] + "..." if len(fd_name) > 30 else fd_name
                output_parts.append(f"file='{fd_short}'")
            
            # 添加网络信息
            if "fd.dip" in sysdig_event:
                output_parts.append(f"net={sysdig_event.get('fd.sip', '')}→{sysdig_event.get('fd.dip', '')}")
            
            return " | ".join(output_parts)
            
        except Exception as e:
            logger.error(f"Error formatting console output: {e}")
            return f"🔄 SYSDIG #{count} | ERROR | {str(e)}"


class SysdigEventFilter(FilterFunction):
    """过滤有效的 Sysdig 事件"""
    
    def filter(self, value):
        return value is not None and len(value.strip()) > 0


def main():
    """主函数：创建 Auditd 到 Sysdig 控制台测试作业"""
    
    logger.info("🚀 Starting SysArmor Auditd to Sysdig Console Test Job")
    logger.info("📋 Based on NODLINK pipeline processing logic")
    
    # 环境变量配置
    kafka_servers = os.getenv('KAFKA_BOOTSTRAP_SERVERS', '49.232.13.155:9094')
    input_topic = 'sysarmor-events-test'  # 固定消费测试 topic
    kafka_group_id = 'sysarmor-auditd-sysdig-console-test-group'
    
    logger.info(f"📡 Kafka Servers: {kafka_servers}")
    logger.info(f"📥 Input Topic: {input_topic}")
    logger.info(f"👥 Consumer Group: {kafka_group_id}")
    logger.info(f"🎯 Output: Console (TaskManager logs)")
    
    # 创建流处理环境
    env = StreamExecutionEnvironment.get_execution_environment()
    
    # 配置环境
    env.set_parallelism(1)  # 单并行度，便于观察输出
    env.enable_checkpointing(60000)  # 60秒 checkpoint
    env.get_checkpoint_config().set_checkpointing_mode(CheckpointingMode.EXACTLY_ONCE)
    
    try:
        # 添加 JAR 依赖
        env.add_jars("file:///opt/flink/lib/flink-sql-connector-kafka-3.1.0-1.18.jar")
        
        # 创建 Kafka Consumer
        kafka_props = {
            'bootstrap.servers': kafka_servers,
            'group.id': kafka_group_id,
            'auto.offset.reset': 'latest',
            'client.dns.lookup': 'use_all_dns_ips',
            'session.timeout.ms': '30000',
            'heartbeat.interval.ms': '10000',
            'max.poll.interval.ms': '300000',
            'connections.max.idle.ms': '540000',
            'request.timeout.ms': '30000'
        }
        
        kafka_consumer = FlinkKafkaConsumer(
            topics=[input_topic],
            deserialization_schema=SimpleStringSchema(),
            properties=kafka_props
        )
        
        logger.info("📋 Creating Auditd to Sysdig conversion pipeline...")
        
        # 构建数据流处理管道
        converted_stream = env.add_source(kafka_consumer) \
            .map(AuditdToSysdigConsoleConverter(), output_type=Types.STRING()) \
            .filter(SysdigEventFilter())
        
        # 输出到控制台
        converted_stream.print()
        
        logger.info("🔄 Auditd to Sysdig conversion pipeline created:")
        logger.info("   Kafka Source (auditd) -> Auditd Parser -> Sysdig Converter -> Console Output")
        logger.info("🎯 NODLINK supported event types:")
        for evt_type in sorted(AuditdSysdigParser.NODLINK_SUPPORTED_EVENTS):
            logger.info(f"   - {evt_type}")
        
        logger.info("🛡️ Features:")
        logger.info("   - Real-time auditd to sysdig conversion")
        logger.info("   - Process tree reconstruction (±60s window)")
        logger.info("   - Hex command line decoding")
        logger.info("   - NODLINK standard event filtering")
        logger.info("   - Console output with formatted display")
        
        # 执行作业
        logger.info("✅ Starting Auditd to Sysdig console test job...")
        
        job_client = env.execute_async("SysArmor-Auditd-Sysdig-Console-Test")
        job_id = job_client.get_job_id()
        
        logger.info(f"🎯 Auditd to Sysdig console test job submitted successfully!")
        logger.info(f"📋 Job ID: {job_id}")
        logger.info(f"🌐 Monitor at: http://localhost:8081")
        logger.info(f"📊 Converting auditd from {input_topic} to sysdig format")
        logger.info(f"🔍 View output: make processor logs-taskmanager")
        
        return job_id
        
    except Exception as e:
        logger.error(f"❌ Auditd to Sysdig console test job failed: {e}")
        raise


if __name__ == "__main__":
    main()
