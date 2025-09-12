#!/usr/bin/env python3
"""
SysArmor Processor - Auditd Raw to Events Job
消费 sysarmor.raw.audit topic，将 auditd 原始数据转换为结构化事件并输出到 sysarmor.events.audit
基于真实的 NODLINK 管道处理逻辑实现
"""

import os
import json
import logging
import re
import socket
from datetime import datetime
from typing import Dict, List, Optional, Any
from collections import defaultdict
from pyflink.datastream import StreamExecutionEnvironment, CheckpointingMode
from pyflink.datastream.connectors.kafka import FlinkKafkaConsumer, FlinkKafkaProducer
from pyflink.common.serialization import SimpleStringSchema
from pyflink.common.typeinfo import Types
from pyflink.datastream.functions import MapFunction, FilterFunction

# 配置日志
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# 基于NODLINK管道的系统调用映射表 (x86_64)
SYSCALL_MAP = {
    # 文件操作
    0: "read", 1: "write", 2: "open", 3: "close", 4: "stat", 5: "fstat", 6: "lstat",
    8: "lseek", 9: "mmap", 10: "mprotect", 11: "munmap", 12: "brk", 
    21: "access", 22: "pipe", 32: "dup", 33: "dup2", 85: "creat", 87: "unlink",
    89: "readlink", 257: "openat", 262: "newfstatat", 
    
    # 进程管理
    39: "getpid", 56: "clone", 57: "fork", 58: "vfork", 59: "execve", 
    60: "exit", 61: "wait4", 231: "exit_group",
    
    # 信号处理
    13: "rt_sigaction", 14: "rt_sigprocmask", 15: "rt_sigreturn",
    
    # 网络操作
    41: "socket", 42: "connect", 43: "accept", 44: "sendto", 45: "recvfrom",
    46: "sendmsg", 47: "recvmsg", 48: "shutdown", 49: "bind", 50: "listen",
    
    # 内存管理
    28: "madvise", 25: "mremap", 73: "munlock", 149: "mlock",
    
    # 时间相关
    96: "gettimeofday", 201: "time", 228: "clock_gettime",
    
    # 权限相关
    105: "setuid", 106: "setgid", 107: "geteuid", 108: "getegid",
    113: "setreuid", 114: "setregid", 117: "setresuid", 119: "setresgid",
    
    # 文件系统
    78: "getcwd", 79: "chdir", 80: "fchdir", 83: "mkdir", 84: "rmdir",
    90: "chmod", 91: "fchmod", 92: "chown", 93: "fchown", 94: "lchown",
    
    # 其他常用系统调用
    63: "uname", 97: "getrlimit", 158: "arch_prctl", 218: "set_tid_address",
    186: "gettid", 272: "unshare", 273: "set_robust_list",
}

# NODLINK标准事件类型（基于原始论文实现）
NODLINK_SUPPORTED_EVENTS = {
    "read", "readv", "write", "writev", "fcntl", "rmdir", "rename", "chmod",
    "execve", "clone", "pipe", "fork", "accept", "sendmsg", "recvmsg", 
    "recvfrom", "send", "sendto", "open", "openat", "socket", "connect"
}

# 事件类别映射
EVENT_CATEGORIES = {
    # 文件操作
    "read": "file", "readv": "file", "write": "file", "writev": "file",
    "open": "file", "openat": "file", "fcntl": "file", "rmdir": "file",
    "rename": "file", "chmod": "file",
    # 进程操作
    "execve": "process", "fork": "process", "clone": "process", "pipe": "process",
    # 网络操作
    "socket": "net", "connect": "net", "accept": "net", "sendto": "net",
    "recvfrom": "net", "sendmsg": "net", "recvmsg": "net", "send": "net",
}

class ProcessTreeBuilder:
    """进程树重建器，用于重建父进程命令行信息"""
    
    def __init__(self, time_window: int = 60):
        self.time_window = time_window
        self.process_cache = {}
        self.system_processes = {
            1: 'systemd --system --deserialize',
            24710: '/usr/sbin/sshd -D',
        }
    
    def add_process(self, pid: int, ppid: int, cmdline: str, timestamp: float):
        """添加进程信息到缓存"""
        if pid and cmdline:
            self.process_cache[pid] = {
                'cmdline': cmdline,
                'ppid': ppid,
                'timestamp': timestamp
            }
            
            # 限制缓存大小
            if len(self.process_cache) > 10000:
                oldest_pids = sorted(self.process_cache.keys(), 
                                   key=lambda pid: self.process_cache[pid]['timestamp'])[:1000]
                for pid in oldest_pids:
                    del self.process_cache[pid]
    
    def get_parent_cmdline(self, ppid: int, event_time: float) -> str:
        """获取父进程命令行"""
        if not ppid:
            return ""
        
        # 1. 时间窗口查找
        for pid, info in self.process_cache.items():
            if (pid == ppid and 
                abs(info['timestamp'] - event_time) <= self.time_window):
                return info['cmdline']
        
        # 2. 系统进程映射
        if ppid in self.system_processes:
            return self.system_processes[ppid]
        
        return ""

class AuditdLogParser:
    """Auditd日志解析器"""
    
    def parse_audit_log_line(self, line: str) -> Optional[Dict[str, Any]]:
        """解析单行auditd日志"""
        line = line.strip()
        if not line:
            return None
        
        # 匹配auditd日志格式
        pattern = r'type=([^ ]+) msg=audit\(([\d.]+):(\d+)\): (.*)'
        match = re.match(pattern, line)
        
        if not match:
            return None
        
        log_type, timestamp, event_id, rest = match.groups()
        
        # 解析字段
        fields = {}
        try:
            for part in re.findall(r'(\w+)=("[^"]*"|\S+)', rest):
                key, val = part
                if val.startswith('"') and val.endswith('"'):
                    val = val[1:-1]
                fields[key] = val
        except Exception:
            return None
        
        return {
            'type': log_type,
            'time': timestamp,
            'event_id': event_id,
            'fields': fields
        }
    
    def decode_cmdline(self, hex_str: str) -> str:
        """解码十六进制编码的命令行"""
        if not hex_str:
            return ""
        
        if not all(c in '0123456789abcdefABCDEF' for c in hex_str):
            return hex_str
        
        if len(hex_str) % 2 != 0:
            return hex_str
        
        try:
            hex_bytes = bytes.fromhex(hex_str)
            parts = hex_bytes.split(b'\x00')
            return b' '.join(part for part in parts if part).decode('utf-8', errors='replace').strip()
        except:
            return hex_str
    
    def extract_exe_name(self, exe_path: str) -> str:
        """提取可执行文件名"""
        if not exe_path:
            return ""
        return os.path.basename(exe_path.strip('"'))

class RawAuditdToEventsConverter(MapFunction):
    """原始 Auditd 数据到结构化事件转换器 - 基于NODLINK管道逻辑"""
    
    def __init__(self):
        self.parser = AuditdLogParser()
        self.tree_builder = ProcessTreeBuilder()
        self.message_count = 0
        self.event_groups = defaultdict(list)  # 按event_id分组
        
    def map(self, value):
        try:
            if not value:
                return None
                
            # 解析输入的原始数据
            raw_data = json.loads(value)
            message = raw_data.get('message', '')
            
            # 解析 auditd 消息
            audit_data = self.parser.parse_audit_log_line(message)
            if not audit_data:
                return None
            
            # 按event_id分组处理
            event_id = audit_data['event_id']
            self.event_groups[event_id].append(audit_data)
            
            # 转换为结构化事件
            structured_event = self._convert_event_group(event_id, self.event_groups[event_id], raw_data)
            if not structured_event:
                return None
            
            self.message_count += 1
            
            # 更新进程缓存
            sysdig_data = structured_event.get('message', {})
            if sysdig_data.get('proc.pid'):
                self.tree_builder.add_process(
                    sysdig_data['proc.pid'],
                    sysdig_data.get('proc.ppid', 0),
                    sysdig_data.get('proc.cmdline', ''),
                    sysdig_data.get('evt.time', 0)
                )
            
            # 重建父进程命令行
            ppid = sysdig_data.get('proc.ppid')
            if ppid:
                pcmdline = self.tree_builder.get_parent_cmdline(
                    ppid, sysdig_data.get('evt.time', 0)
                )
                structured_event['message']['proc.pcmdline'] = pcmdline
            
            # 返回JSON字符串
            return json.dumps(structured_event, ensure_ascii=False)
            
        except Exception as e:
            logger.error(f"Error in RawAuditdToEventsConverter: {e}")
            return None
    
    def _convert_event_group(self, event_id: str, entries: List[Dict[str, Any]], 
                           raw_data: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        """转换事件组为SysArmor扩展格式 - 包装sysdig数据"""
        if not entries:
            return None
        
        # 先构建标准sysdig格式
        sysdig_event = {
            "evt.num": int(event_id),
            "evt.time": 0,
            "evt.type": "unknown",
            "evt.category": "other",
            "evt.dir": ">",
            "evt.args": "",
            "proc.name": "",
            "proc.exe": "",
            "proc.cmdline": "",
            "proc.pid": None,
            "proc.ppid": None,
            "proc.pcmdline": "",  # 父进程命令行，将通过进程树重建
            "proc.uid": None,
            "proc.gid": None,
            "fd.name": "",
            "net.sockaddr": {},
            "host": raw_data.get('host', 'unknown'),
            "is_warn": False
        }
        
        # 处理各种类型的audit记录
        for entry in entries:
            entry_type = entry['type']
            fields = entry['fields']
            
            if entry_type == 'SYSCALL':
                self._process_syscall_entry(sysdig_event, fields, entry)
            elif entry_type == 'PATH':
                self._process_path_entry(sysdig_event, fields)
            elif entry_type == 'PROCTITLE':
                self._process_proctitle_entry(sysdig_event, fields)
            elif entry_type == 'SOCKADDR':
                self._process_sockaddr_entry(sysdig_event, fields)
        
        # 只处理NODLINK支持的事件类型
        if sysdig_event["evt.type"] not in NODLINK_SUPPORTED_EVENTS:
            return None
        
        # 设置事件类别
        if sysdig_event["evt.type"] in EVENT_CATEGORIES:
            sysdig_event["evt.category"] = EVENT_CATEGORIES[sysdig_event["evt.type"]]
        
        # 构建SysArmor扩展格式 - 将sysdig数据包装在sysdig字段中
        result = {
            # SysArmor元数据
            "event_id": event_id,
            "timestamp": datetime.fromtimestamp(sysdig_event["evt.time"]).isoformat() + 'Z',
            "collector_id": raw_data.get('collector_id', ''),
            "host": raw_data.get('host', 'unknown'),
            "source": "auditd",
            "processor": "flink-nodlink-converter",
            "processed_at": datetime.utcnow().isoformat() + 'Z',
            
            # 事件分类
            "event_type": sysdig_event["evt.type"],
            "event_category": sysdig_event["evt.category"],
            "severity": "low",  # 默认严重程度
            
            # 完整的sysdig格式数据 (改名为message以保持API兼容性)
            "message": sysdig_event
        }
        
        return result
    
    def _process_syscall_entry(self, result: Dict[str, Any], fields: Dict[str, str], entry: Dict[str, Any]):
        """处理SYSCALL类型的记录 - sysdig格式"""
        # 时间戳
        result["evt.time"] = float(entry['time'])
        
        # 系统调用信息
        if 'syscall' in fields:
            try:
                syscall_num = int(fields['syscall'])
                result['evt.type'] = SYSCALL_MAP.get(syscall_num, f"syscall_{syscall_num}")
            except ValueError:
                result['evt.type'] = f"syscall_{fields['syscall']}"
        
        # 进程信息 - sysdig格式
        if 'exe' in fields:
            result['proc.exe'] = fields['exe'].strip('"')
            result['proc.name'] = self.parser.extract_exe_name(fields['exe'])
        
        if 'pid' in fields:
            try:
                result['proc.pid'] = int(fields['pid'])
            except ValueError:
                pass
        
        if 'ppid' in fields:
            try:
                result['proc.ppid'] = int(fields['ppid'])
            except ValueError:
                pass
        
        # 用户信息
        if 'uid' in fields:
            try:
                result['proc.uid'] = int(fields['uid'])
            except ValueError:
                pass
        
        if 'gid' in fields:
            try:
                result['proc.gid'] = int(fields['gid'])
            except ValueError:
                pass
        
        # is_warn字段 - 在NODLINK中用于标识异常事件，实时处理时默认为False
        # 注意：这不是系统调用成功与否的标识，而是NODLINK算法的标签字段
        # 在后续的异常检测阶段会根据可疑进程等规则来设置这个字段
        result['is_warn'] = False
        
        # 构建参数字符串
        excluded_keys = {'key', 'syscall', 'exe', 'pid', 'ppid', 'uid', 'gid', 'success', 'exit'}
        args = []
        for k, v in fields.items():
            if k not in excluded_keys:
                args.append(f"{k}={v}")
        result['evt.args'] = " ".join(args)
    
    def _process_path_entry(self, result: Dict[str, Any], fields: Dict[str, str]):
        """处理PATH类型的记录 - sysdig格式"""
        if 'name' in fields:
            file_path = fields['name'].strip('"')
            if file_path and file_path != '(null)':
                if result['fd.name']:
                    result['fd.name'] += f",{file_path}"
                else:
                    result['fd.name'] = file_path
    
    def _process_proctitle_entry(self, result: Dict[str, Any], fields: Dict[str, str]):
        """处理PROCTITLE类型的记录 - sysdig格式"""
        if 'proctitle' in fields:
            cmdline = self.parser.decode_cmdline(fields['proctitle'])
            if cmdline:
                result['proc.cmdline'] = cmdline
    
    def _process_sockaddr_entry(self, result: Dict[str, Any], fields: Dict[str, str]):
        """处理SOCKADDR类型的记录 - sysdig格式"""
        if 'saddr' in fields:
            try:
                sockaddr_info = self._parse_sockaddr(fields['saddr'])
                if sockaddr_info:
                    result['net.sockaddr'] = sockaddr_info
            except Exception as e:
                logger.debug(f"Failed to parse sockaddr: {e}")
    
    def _parse_sockaddr(self, hex_str: str) -> Dict[str, Any]:
        """解析网络地址信息"""
        try:
            if len(hex_str) < 4:
                return {"family": "unknown", "address": hex_str}
            
            # 前两个字节是协议族（小端序）
            family_hex = hex_str[:4]
            family = int.from_bytes(bytes.fromhex(family_hex), byteorder="little")
            
            if family == 2:  # AF_INET
                if len(hex_str) >= 16:
                    port_bytes = bytes.fromhex(hex_str[4:8])
                    port = int.from_bytes(port_bytes, byteorder="big")
                    ip_bytes = bytes.fromhex(hex_str[8:16])
                    ip = socket.inet_ntop(socket.AF_INET, ip_bytes)
                    return {
                        "family": "AF_INET",
                        "type": "ipv4",
                        "source_ip": ip,
                        "source_port": port,
                        "address": f"{ip}:{port}"
                    }
            
            return {"family": f"family_{family}", "address": hex_str}
            
        except Exception:
            return {"family": "error", "address": hex_str}

class ValidStructuredEventFilter(FilterFunction):
    """过滤有效的结构化事件"""
    
    def filter(self, value):
        if not value:
            return False
        
        try:
            event = json.loads(value)
            # 确保必要字段存在且事件类型被支持
            required_fields = ["event_id", "timestamp", "collector_id", "event_type"]
            has_required = all(field in event for field in required_fields)
            is_supported = event.get("event_type") in NODLINK_SUPPORTED_EVENTS
            return has_required and is_supported
        except:
            return False

def main():
    """主函数：创建 Auditd Raw to Events 处理作业"""
    
    logger.info("🚀 Starting SysArmor Auditd Raw to Events Job")
    logger.info("📋 Based on NODLINK pipeline processing logic")
    logger.info("📊 Processing: sysarmor.raw.audit → sysarmor.events.audit")
    
    # 环境变量配置
    kafka_servers = os.getenv('KAFKA_BOOTSTRAP_SERVERS', 'middleware-kafka:9092')
    input_topic = 'sysarmor.raw.audit'
    output_topic = 'sysarmor.events.audit'
    kafka_group_id = 'sysarmor-auditd-raw-to-events-processor'
    
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
            'auto.offset.reset': 'earliest',
            'session.timeout.ms': '30000',
            'heartbeat.interval.ms': '10000',
            'max.poll.interval.ms': '300000'
        }
        
        kafka_consumer = FlinkKafkaConsumer(
            topics=[input_topic],
            deserialization_schema=SimpleStringSchema(),
            properties=consumer_props
        )
        
        # 创建 Kafka Producer
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
        
        logger.info("📋 Creating NODLINK-based processing pipeline...")
        
        # 构建数据流处理管道
        processed_stream = env.add_source(kafka_consumer) \
            .map(RawAuditdToEventsConverter(), output_type=Types.STRING()) \
            .filter(ValidStructuredEventFilter())
        
        # 输出到目标topic
        processed_stream.add_sink(kafka_producer)
        
        # 监控输出
        processed_stream.map(
            lambda x: f"✅ Processed: {json.loads(x).get('event_type', 'unknown')} from {json.loads(x).get('collector_id', 'unknown')[:8]}",
            output_type=Types.STRING()
        ).print()
        
        logger.info("🔄 NODLINK-based Auditd processing pipeline created:")
        logger.info(f"   {input_topic} -> Auditd Parser -> Event Grouping -> Sysdig Conversion -> Process Tree Rebuild -> {output_topic}")
        logger.info("🎯 NODLINK supported event types:")
        for evt_type in sorted(NODLINK_SUPPORTED_EVENTS):
            logger.info(f"   - {evt_type}")
        
        logger.info("🛡️ Features:")
        logger.info("   - Real-time auditd parsing (SYSCALL/PATH/PROCTITLE/SOCKADDR)")
        logger.info("   - Process tree reconstruction with 60s time window")
        logger.info("   - Hex command line decoding")
        logger.info("   - NODLINK standard event filtering")
        logger.info("   - Network address parsing")
        
        # 执行作业
        logger.info("✅ Starting NODLINK-based Auditd processing job...")
        
        job_client = env.execute_async("SysArmor-NODLINK-Auditd-Raw-to-Events")
        job_id = job_client.get_job_id()
        
        logger.info(f"🎯 NODLINK Auditd processing job submitted successfully!")
        logger.info(f"📋 Job ID: {job_id}")
        logger.info(f"🌐 Monitor at: http://localhost:8081")
        logger.info(f"📊 Processing: {input_topic} → {output_topic}")
        logger.info(f"🔍 View logs: docker logs -f sysarmor-flink-taskmanager-1")
        
        return job_id
        
    except Exception as e:
        logger.error(f"❌ NODLINK Auditd processing job failed: {e}")
        raise

if __name__ == "__main__":
    main()
