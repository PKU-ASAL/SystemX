#!/usr/bin/env python3
"""
SysArmor Processor - Auditd to Sysdig Format Converter
将auditd格式的数据转换为sysdig格式，用于后续的威胁检测和分析
"""

import os
import json
import logging
import re
from datetime import datetime
from typing import Dict, List, Optional, Any
from pyflink.datastream import StreamExecutionEnvironment, CheckpointingMode
from pyflink.datastream.connectors.kafka import FlinkKafkaConsumer, FlinkKafkaProducer
from pyflink.common.serialization import SimpleStringSchema
from pyflink.common.typeinfo import Types
from pyflink.datastream.functions import MapFunction, FilterFunction
from pyflink.common import Configuration

# 配置日志
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class AuditdParser:
    """Auditd日志解析器"""
    
    # 系统调用号到事件类型的映射
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
    
    # NODLINK支持的事件类型
    SUPPORTED_EVENTS = {
        "read", "readv", "write", "writev", "fcntl", "rmdir", "rename", "chmod",
        "execve", "clone", "pipe", "fork", "accept", "sendmsg", "recvmsg", 
        "recvfrom", "send", "sendto", "open", "openat", "socket", "connect"
    }
    
    def __init__(self):
        self.process_cache = {}  # 进程缓存，用于进程树重建
        
    def parse_auditd_line(self, line: str) -> Optional[Dict[str, Any]]:
        """解析单行auditd日志"""
        try:
            # 正则匹配auditd格式
            match = re.match(r'type=([^ ]+) msg=audit\(([\d.]+):(\d+)\): (.*)', line)
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
            logger.warning(f"Failed to parse auditd line: {line}, error: {e}")
            return None
    
    def decode_cmdline(self, hex_str: str) -> str:
        """解码命令行（可能是十六进制编码）"""
        if not hex_str:
            return ""
        
        # 检查是否为十六进制字符串
        if not all(c in '0123456789abcdefABCDEF' for c in hex_str):
            return hex_str  # 非十六进制，直接返回
        
        if len(hex_str) % 2 != 0:
            return hex_str  # 长度必须为偶数
        
        try:
            hex_bytes = bytes.fromhex(hex_str)
            parts = hex_bytes.split(b'\x00')
            return b' '.join(part for part in parts if part).decode('utf-8', errors='replace').strip()
        except:
            return hex_str  # 解码失败，返回原字符串
    
    def convert_to_sysdig(self, audit_records: List[Dict[str, Any]]) -> Optional[Dict[str, Any]]:
        """将分组的audit记录转换为sysdig格式"""
        try:
            # 查找SYSCALL记录
            syscall_record = None
            path_records = []
            proctitle_record = None
            
            for record in audit_records:
                if record['type'] == 'SYSCALL':
                    syscall_record = record
                elif record['type'] == 'PATH':
                    path_records.append(record)
                elif record['type'] == 'PROCTITLE':
                    proctitle_record = record
            
            if not syscall_record:
                return None
            
            fields = syscall_record['fields']
            
            # 获取系统调用类型
            syscall_num = int(fields.get('syscall', '0'))
            evt_type = self.SYSCALL_MAP.get(syscall_num, f"syscall_{syscall_num}")
            
            # 只处理支持的事件类型
            if evt_type not in self.SUPPORTED_EVENTS:
                return None
            
            # 构建sysdig格式事件
            sysdig_event = {
                "evt.num": int(syscall_record['event_id']),
                "evt.time": syscall_record['timestamp'],
                "evt.type": evt_type,
                "evt.category": self._get_event_category(evt_type),
                "proc.name": fields.get('comm', '').strip('"'),
                "proc.exe": fields.get('exe', '').strip('"'),
                "proc.pid": int(fields.get('pid', '0')),
                "proc.ppid": int(fields.get('ppid', '0')),
                "is_warn": False
            }
            
            # 处理命令行
            if proctitle_record and 'proctitle' in proctitle_record['fields']:
                cmdline = self.decode_cmdline(proctitle_record['fields']['proctitle'])
                sysdig_event["proc.cmdline"] = cmdline
            else:
                sysdig_event["proc.cmdline"] = sysdig_event["proc.name"]
            
            # 处理文件路径
            if path_records:
                # 取第一个有效路径
                for path_record in path_records:
                    if 'name' in path_record['fields']:
                        fd_name = path_record['fields']['name'].strip('"')
                        if fd_name and fd_name != '(null)':
                            sysdig_event["fd.name"] = fd_name
                            break
            
            # 处理网络事件
            if evt_type in ['socket', 'connect', 'accept', 'sendto', 'recvfrom']:
                self._add_network_fields(sysdig_event, fields)
            
            # 更新进程缓存
            self._update_process_cache(sysdig_event)
            
            # 添加父进程命令行
            parent_cmdline = self._get_parent_cmdline(
                sysdig_event["proc.ppid"], 
                sysdig_event["evt.time"]
            )
            sysdig_event["proc.pcmdline"] = parent_cmdline
            
            return sysdig_event
            
        except Exception as e:
            logger.error(f"Failed to convert audit records to sysdig: {e}")
            return None
    
    def _get_event_category(self, evt_type: str) -> str:
        """获取事件类别"""
        file_events = {"read", "readv", "write", "writev", "open", "openat", "close", "fcntl"}
        process_events = {"execve", "clone", "fork", "exit"}
        network_events = {"socket", "connect", "accept", "sendto", "recvfrom", "sendmsg", "recvmsg"}
        
        if evt_type in file_events:
            return "file"
        elif evt_type in process_events:
            return "process"
        elif evt_type in network_events:
            return "net"
        else:
            return "other"
    
    def _add_network_fields(self, event: Dict[str, Any], fields: Dict[str, str]):
        """添加网络相关字段"""
        # 这里可以根据需要添加网络地址、端口等信息
        if 'saddr' in fields:
            event["fd.sip"] = fields['saddr']
        if 'daddr' in fields:
            event["fd.dip"] = fields['daddr']
        if 'sport' in fields:
            event["fd.sport"] = int(fields['sport'])
        if 'dport' in fields:
            event["fd.dport"] = int(fields['dport'])
    
    def _update_process_cache(self, event: Dict[str, Any]):
        """更新进程缓存"""
        pid = event["proc.pid"]
        self.process_cache[pid] = {
            'cmdline': event["proc.cmdline"],
            'timestamp': event["evt.time"]
        }
    
    def _get_parent_cmdline(self, ppid: int, event_time: float) -> str:
        """获取父进程命令行"""
        if not ppid:
            return ""
        
        # 系统进程映射
        system_processes = {
            1: 'systemd --system --deserialize',
            2: '[kthreadd]',
            0: ''
        }
        
        if ppid in system_processes:
            return system_processes[ppid]
        
        # 从缓存中查找
        if ppid in self.process_cache:
            cached_info = self.process_cache[ppid]
            # 检查时间窗口（±60秒）
            if abs(cached_info['timestamp'] - event_time) <= 60:
                return cached_info['cmdline']
        
        return ""  # 无法重建


class AuditdToSysdigConverter(MapFunction):
    """Auditd到Sysdig格式转换器"""
    
    def __init__(self):
        self.parser = AuditdParser()
        self.event_buffer = {}  # 按event_id分组的缓冲区
        
    def map(self, value):
        try:
            if not value:
                return None
                
            # 解析输入的JSON消息
            data = json.loads(value)
            message = data.get('message', '')
            host = data.get('host', 'unknown')
            
            # 解析auditd日志行
            audit_record = self.parser.parse_auditd_line(message)
            if not audit_record:
                return None
            
            event_id = audit_record['event_id']
            
            # 将记录添加到缓冲区
            if event_id not in self.event_buffer:
                self.event_buffer[event_id] = []
            self.event_buffer[event_id].append(audit_record)
            
            # 检查是否可以处理这个事件组
            # 简化处理：如果有SYSCALL记录就尝试转换
            has_syscall = any(r['type'] == 'SYSCALL' for r in self.event_buffer[event_id])
            
            if has_syscall:
                # 转换为sysdig格式
                sysdig_event = self.parser.convert_to_sysdig(self.event_buffer[event_id])
                
                # 清理缓冲区
                del self.event_buffer[event_id]
                
                if sysdig_event:
                    # 添加主机信息
                    sysdig_event["host"] = host
                    
                    logger.debug(f"Converted event: {sysdig_event['evt.type']} from {sysdig_event['proc.name']}")
                    return json.dumps(sysdig_event, ensure_ascii=False)
            
            return None
            
        except Exception as e:
            logger.error(f"Error in AuditdToSysdigConverter: {e}")
            return None


class SysdigEventFilter(FilterFunction):
    """过滤有效的Sysdig事件"""
    
    def filter(self, value):
        try:
            if not value:
                return False
            
            event = json.loads(value)
            
            # 检查必需字段
            required_fields = ['evt.type', 'proc.pid', 'proc.name']
            for field in required_fields:
                if field not in event:
                    return False
            
            # 检查事件类型是否支持
            if event['evt.type'] not in AuditdParser.SUPPORTED_EVENTS:
                return False
            
            return True
            
        except Exception as e:
            logger.error(f"Error in SysdigEventFilter: {e}")
            return False


def generate_output_topic(input_topic: str) -> str:
    """根据输入topic生成对应的输出topic"""
    # 将 sysarmor-agentless-xxx 转换为 sysarmor-sysdig-xxx
    if input_topic.startswith('sysarmor-agentless-'):
        collector_id = input_topic.replace('sysarmor-agentless-', '')
        return f'sysarmor-sysdig-{collector_id}'
    else:
        # 如果不是标准格式，添加-sysdig后缀
        return f'{input_topic}-sysdig'

def main():
    """主函数：创建Auditd到Sysdig转换作业"""
    
    logger.info("🚀 Starting SysArmor Auditd to Sysdig Converter Job")
    
    # 环境变量
    kafka_servers = os.getenv('KAFKA_BOOTSTRAP_SERVERS', 'middleware-kafka:9092')
    input_topic = os.getenv('INPUT_TOPIC', 'sysarmor-agentless-558c01dd')
    
    # 如果没有明确指定输出topic，则根据输入topic自动生成
    output_topic = os.getenv('OUTPUT_TOPIC')
    if not output_topic:
        output_topic = generate_output_topic(input_topic)
        logger.info(f"🔄 Auto-generated output topic: {output_topic}")
    
    kafka_group_id = os.getenv('KAFKA_GROUP_ID', 'sysarmor-auditd-converter-group')
    
    logger.info(f"📡 Kafka Servers: {kafka_servers}")
    logger.info(f"📥 Input Topic: {input_topic}")
    logger.info(f"📤 Output Topic: {output_topic}")
    logger.info(f"👥 Consumer Group: {kafka_group_id}")
    
    # 创建流处理环境
    env = StreamExecutionEnvironment.get_execution_environment()
    
    # 配置环境
    env.set_parallelism(2)  # 设置并行度
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
        
        # 创建 Kafka Producer
        kafka_producer = FlinkKafkaProducer(
            topic=output_topic,
            serialization_schema=SimpleStringSchema(),
            producer_config=kafka_props
        )
        
        logger.info("📋 Creating conversion pipeline...")
        
        # 构建数据流处理管道
        converted_stream = env.add_source(kafka_consumer) \
            .map(AuditdToSysdigConverter(), output_type=Types.STRING()) \
            .filter(SysdigEventFilter())
        
        # 输出到Kafka
        converted_stream.add_sink(kafka_producer)
        
        logger.info("🔄 Conversion pipeline created:")
        logger.info("   Kafka Source (Auditd) -> Auditd Parser -> Sysdig Converter -> Filter -> Kafka Sink (Sysdig)")
        logger.info("🎯 Supported event types:")
        for evt_type in sorted(AuditdParser.SUPPORTED_EVENTS):
            logger.info(f"   - {evt_type}")
        
        logger.info("🛡️ Features:")
        logger.info("   - Real-time auditd to sysdig conversion")
        logger.info("   - Process tree reconstruction")
        logger.info("   - Command line decoding")
        logger.info("   - Event filtering and validation")
        
        # 执行作业
        logger.info("✅ Starting Auditd to Sysdig conversion job...")
        
        job_client = env.execute_async("SysArmor-Auditd-To-Sysdig-Converter")
        job_id = job_client.get_job_id()
        
        logger.info(f"🎯 Conversion job submitted successfully!")
        logger.info(f"📋 Job ID: {job_id}")
        logger.info(f"🌐 Monitor at: http://localhost:8081")
        logger.info(f"📊 Converting from {input_topic} to {output_topic}")
        
        return job_id
        
    except Exception as e:
        logger.error(f"❌ Conversion job failed: {e}")
        raise


if __name__ == "__main__":
    main()
