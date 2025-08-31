#!/usr/bin/env python3
"""
SysArmor Processor - DataStream API Version
使用 DataStream API 替代 Table API，提供更好的错误处理和调试能力
"""

import os
import json
import logging
from datetime import datetime
from pyflink.datastream import StreamExecutionEnvironment, CheckpointingMode
from pyflink.datastream.connectors.kafka import FlinkKafkaConsumer
from pyflink.common.serialization import SimpleStringSchema
from pyflink.common.typeinfo import Types
from pyflink.datastream.functions import MapFunction, FilterFunction, SinkFunction
from pyflink.common import Configuration
import requests

# 配置日志
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class ThreatEvent:
    """威胁事件数据类 - 保留完整的原始字段信息"""
    def __init__(self, timestamp, host, message, threat_type, risk_score, severity, 
                 collector_id=None, source_type=None, program=None, original_data=None):
        self.timestamp = timestamp
        self.host = host
        self.message = message
        self.threat_type = threat_type
        self.risk_score = risk_score
        self.severity = severity
        self.collector_id = collector_id
        self.source_type = source_type
        self.program = program
        self.original_data = original_data or {}
        
    def to_dict(self):
        # 构建完整的威胁事件，保留所有原始字段（移除host字段避免冲突）
        threat_event = {
            '@timestamp': self.timestamp,
            'hostname': self.host,  # 只保留 hostname 字段
            'message': self.message,
            'threat_type': self.threat_type,
            'risk_score': self.risk_score,
            'severity': self.severity,
            'event_type': 'threat_detection',  # 标识为威胁检测事件
        }
        
        # 添加数据源信息
        if self.collector_id:
            threat_event['collector_id'] = self.collector_id
            threat_event['data_source'] = self.collector_id  # 兼容前端字段名
            
        if self.source_type:
            threat_event['source_type'] = self.source_type
            
        if self.program:
            threat_event['program'] = self.program
            
        # 保留原始事件的其他有用字段
        if self.original_data:
            for key in ['port', 'processed_at', 'topic']:
                if key in self.original_data:
                    threat_event[key] = self.original_data[key]
        
        return threat_event

class JsonParser(MapFunction):
    """JSON 解析器 - 更安全的错误处理"""
    
    def map(self, value):
        try:
            data = json.loads(value)
            logger.debug(f"Parsed JSON: {data}")
            return data
        except json.JSONDecodeError as e:
            logger.warning(f"Failed to parse JSON: {value}, error: {e}")
            # 返回一个默认结构，避免作业失败
            return {
                'event_type': 'unknown',
                'message': value,
                'host': 'unknown',
                'timestamp': datetime.now().isoformat()
            }
        except Exception as e:
            logger.error(f"Unexpected error parsing JSON: {e}")
            return {
                'event_type': 'error',
                'message': str(value),
                'host': 'unknown',
                'timestamp': datetime.now().isoformat()
            }

class SyslogFilter(FilterFunction):
    """Syslog 事件过滤器"""
    
    def filter(self, value):
        try:
            if isinstance(value, dict):
                event_type = value.get('event_type', '')
                message = value.get('message', '')
                
                # 只处理 syslog 事件且包含威胁关键词
                is_syslog = event_type == 'syslog'
                has_threat_keywords = any(keyword in message.lower() for keyword in [
                    'sudo', 'rm -rf', 'netcat', 'nc -', 'nmap', 'chmod 777'
                ])
                
                result = is_syslog and has_threat_keywords
                if result:
                    logger.info(f"Threat detected: {message[:100]}...")
                
                return result
            return False
        except Exception as e:
            logger.error(f"Error in filter: {e}")
            return False

class ThreatDetector(MapFunction):
    """威胁检测器 - 增强的连续 sudo 检测，保留完整原始字段"""
    
    def __init__(self):
        self.sudo_count = {}  # 记录每个主机的 sudo 计数
        
    def map(self, value):
        try:
            if not isinstance(value, dict):
                return None
                
            # 提取原始事件字段
            message = value.get('message', '')
            host = value.get('host', 'unknown')
            collector_id = value.get('collector_id', None)
            source_type = value.get('source_type', None)
            program = value.get('program', None)
            timestamp = value.get('timestamp', datetime.now().isoformat())
            
            # 检测威胁类型和风险评分
            threat_type = 'unknown'
            risk_score = 60
            severity = 'low'
            
            message_lower = message.lower()
            
            # 连续 sudo 检测逻辑
            if 'sudo' in message_lower:
                # 增加该主机的 sudo 计数
                if host not in self.sudo_count:
                    self.sudo_count[host] = 0
                self.sudo_count[host] += 1
                
                logger.info(f"Host {host} sudo count: {self.sudo_count[host]}")
                
                # 根据连续次数调整风险评分
                if self.sudo_count[host] >= 2:
                    threat_type = 'consecutive_sudo'
                    risk_score = 95
                    severity = 'critical'
                    logger.warning(f"CRITICAL: Consecutive sudo detected on {host} (count: {self.sudo_count[host]})")
                else:
                    threat_type = 'privilege_escalation'
                    risk_score = 85
                    severity = 'high'
                    
            elif 'rm -rf' in message_lower:
                threat_type = 'file_deletion'
                risk_score = 90
                severity = 'critical'
                
            elif any(nc in message_lower for nc in ['netcat', 'nc -']):
                threat_type = 'command_injection'
                risk_score = 95
                severity = 'critical'
                
            elif 'nmap' in message_lower:
                threat_type = 'network_scanning'
                risk_score = 70
                severity = 'medium'
                
            elif 'chmod 777' in message_lower:
                threat_type = 'permission_change'
                risk_score = 75
                severity = 'high'
            
            # 创建威胁事件，传递完整的原始字段信息
            threat_event = ThreatEvent(
                timestamp=timestamp,
                host=host,
                message=message,
                threat_type=threat_type,
                risk_score=risk_score,
                severity=severity,
                collector_id=collector_id,
                source_type=source_type,
                program=program,
                original_data=value  # 传递完整的原始数据
            )
            
            logger.info(f"Threat detected: {threat_type} (score: {risk_score}) on {host} from collector {collector_id}")
            return threat_event.to_dict()
            
        except Exception as e:
            logger.error(f"Error in threat detection: {e}")
            return None

class PrintSink(MapFunction):
    """打印输出 Sink - 用于调试"""
    
    def map(self, value):
        try:
            if value:
                print(f"🚨 THREAT_DETECTED: {json.dumps(value, ensure_ascii=False)}")
                logger.info(f"Threat output: {value}")
            return value
        except Exception as e:
            logger.error(f"Error in print sink: {e}")
            return value

class OpenSearchSink(MapFunction):
    """OpenSearch HTTP Sink - 使用 HTTP API 写入威胁数据"""
    
    def __init__(self):
        self.opensearch_host = os.getenv('OPENSEARCH_HOST', 'localhost')
        self.opensearch_port = os.getenv('OPENSEARCH_PORT', '9201')
        self.opensearch_username = os.getenv('OPENSEARCH_USERNAME', 'admin')
        self.opensearch_password = os.getenv('OPENSEARCH_PASSWORD', 'admin')
        self.threats_index = os.getenv('THREATS_INDEX', 'sysarmor-threats')
        
        self.base_url = f"http://{self.opensearch_host}:{self.opensearch_port}"
        self.index_url = f"{self.base_url}/{self.threats_index}/_doc"
        
    def map(self, value):
        """写入威胁事件到 OpenSearch"""
        try:
            if not value:
                return value
                
            # 准备 HTTP 请求
            headers = {
                'Content-Type': 'application/json'
            }
            
            auth = (self.opensearch_username, self.opensearch_password)
            
            # 发送 POST 请求到 OpenSearch
            response = requests.post(
                self.index_url,
                json=value,
                headers=headers,
                auth=auth,
                timeout=10,
                verify=False  # 忽略 SSL 证书验证
            )
            
            if response.status_code in [200, 201]:
                logger.info(f"✅ Threat event written to OpenSearch: {value.get('threat_type', 'unknown')}")
            else:
                logger.error(f"❌ Failed to write to OpenSearch: {response.status_code} - {response.text}")
                
        except requests.exceptions.RequestException as e:
            logger.error(f"❌ OpenSearch connection error: {e}")
        except Exception as e:
            logger.error(f"❌ Unexpected error in OpenSearch sink: {e}")
        
        return value

def main():
    """主函数：使用 DataStream API 创建威胁检测作业"""
    
    logger.info("🚀 Starting SysArmor Threat Detection Job (DataStream API)")
    
    # 环境变量
    kafka_servers = os.getenv('KAFKA_BOOTSTRAP_SERVERS', '101.42.117.44:9093')
    kafka_group_id = os.getenv('KAFKA_GROUP_ID', 'sysarmor-datastream-group')
    
    logger.info(f"📡 Kafka Servers: {kafka_servers}")
    logger.info(f"👥 Consumer Group: {kafka_group_id}")
    
    # 创建流处理环境
    env = StreamExecutionEnvironment.get_execution_environment()
    
    # 配置环境
    env.set_parallelism(1)  # 单并行度，便于调试
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
            'request.timeout.ms': '30000',
            # 强制使用 bootstrap servers，避免 advertised.listeners 问题
            'metadata.max.age.ms': '30000',
            'reconnect.backoff.ms': '1000',
            'retry.backoff.ms': '1000'
        }
        
        kafka_consumer = FlinkKafkaConsumer(
            topics=['sysarmor-agentless-558c01dd', 'sysarmor-agentless-7bb885a8'],
            deserialization_schema=SimpleStringSchema(),
            properties=kafka_props
        )
        
        logger.info("📋 Creating Kafka source...")
        
        # 构建数据流处理管道
        threat_stream = env.add_source(kafka_consumer) \
            .map(JsonParser(), output_type=Types.PICKLED_BYTE_ARRAY()) \
            .filter(SyslogFilter()) \
            .map(ThreatDetector(), output_type=Types.PICKLED_BYTE_ARRAY()) \
            .filter(lambda x: x is not None)
        
        # 分流：同时输出到控制台和 OpenSearch
        threat_stream.map(PrintSink(), output_type=Types.PICKLED_BYTE_ARRAY())
        threat_stream.map(OpenSearchSink(), output_type=Types.PICKLED_BYTE_ARRAY())
        
        logger.info("🔍 DataStream pipeline created:")
        logger.info("   Kafka Source -> JSON Parser -> Syslog Filter -> Threat Detector -> [Print Sink + OpenSearch Sink]")
        logger.info("🎯 Detection rules:")
        logger.info("   - Single sudo: Risk 85 (HIGH)")
        logger.info("   - Consecutive sudo: Risk 95 (CRITICAL)")
        logger.info("   - File deletion: Risk 90 (CRITICAL)")
        logger.info("   - Command injection: Risk 95 (CRITICAL)")
        logger.info("   - Network scanning: Risk 70 (MEDIUM)")
        logger.info("   - Permission change: Risk 75 (HIGH)")
        
        logger.info("🛡️ Enhanced features:")
        logger.info("   - DataStream API for better error handling")
        logger.info("   - Stateful consecutive sudo detection")
        logger.info("   - Comprehensive logging and debugging")
        logger.info("   - Graceful error recovery")
        
        # 执行作业 - 异步提交
        logger.info("✅ Starting DataStream threat detection job...")
        
        # 获取作业执行结果但不等待完成
        job_client = env.execute_async("SysArmor-DataStream-Threat-Detection")
        job_id = job_client.get_job_id()
        
        logger.info(f"🎯 DataStream job submitted successfully!")
        logger.info(f"📋 Job ID: {job_id}")
        logger.info(f"🌐 Monitor at: http://localhost:8081")
        logger.info(f"🚨 Threats will be printed to TaskManager logs")
        logger.info(f"📊 Job is running in background...")
        
        # 不等待作业完成，让脚本正常退出
        return job_id
        
    except Exception as e:
        logger.error(f"❌ DataStream job failed: {e}")
        raise

if __name__ == "__main__":
    main()
