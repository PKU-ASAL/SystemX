#!/usr/bin/env python3
"""
SysArmor Processor - Events to Alerts Job
消费 sysarmor.events.audit topic，基于威胁检测规则过滤出告警事件
输出到两个 Kafka topics:
  1. sysarmor.alerts.audit - 告警事件
  2. sysarmor.inference.requests - 规范化的推理请求（供job03消费）
基于 Falco/Sysdig 规则引擎设计
"""

import os
import json
import logging
import requests
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

# 导入威胁检测引擎（复用模块）
from threat_detection_engine import ThreatDetectionRules

# 配置日志
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# 全局威胁检测引擎实例
_global_rules_engine = None

def get_rules_engine():
    """获取全局威胁检测引擎实例"""
    global _global_rules_engine
    if _global_rules_engine is None:
        threat_logger = logging.getLogger('threat_detection_engine')
        original_level = threat_logger.level
        threat_logger.setLevel(logging.WARNING)
        
        _global_rules_engine = ThreatDetectionRules()
        
        # 恢复日志级别
        threat_logger.setLevel(original_level)
        
        logger.info(f"✅ 威胁检测引擎已初始化: {len(_global_rules_engine.rules)} 个规则")
    return _global_rules_engine


class EventNormalizer:
    """事件规范化器 - 将原始事件转换为推理服务所需格式"""
    
    @staticmethod
    def normalize_event(event: Dict[str, Any], is_warn: bool) -> Dict[str, Any]:
        """
        将事件规范化为推理服务格式
        """
        try:
            # 获取message对象（包含详细事件信息）
            message = event.get('message', {})
            
            # 提取事件时间 - 优先使用顶层timestamp，格式化为字符串
            evt_time = event.get('timestamp', '')
            if not evt_time and 'evt.time' in message:
                # 如果没有timestamp，使用message中的evt.time（Unix时间戳）
                evt_time_unix = message.get('evt.time', '')
                if evt_time_unix:
                    try:
                        from datetime import datetime
                        evt_time = datetime.utcfromtimestamp(float(evt_time_unix)).isoformat() + 'Z'
                    except:
                        evt_time = str(evt_time_unix)
            
            # 提取事件类型 - 优先使用顶层event_type
            evt_type = event.get('event_type', message.get('evt.type', ''))
            
            # 提取进程信息 - 从message中获取
            proc_cmdline = message.get('proc.cmdline', '')
            proc_pid = str(message.get('proc.pid', ''))
            proc_ppid = str(message.get('proc.ppid', ''))
            proc_pcmdline = message.get('proc.pcmdline', '')
            
            # 提取文件描述符信息 - 从message中获取
            fd_name = message.get('fd.name', '')
            
            # 提取主机信息 - 优先使用顶层host
            host = event.get('host', message.get('host', ''))
            
            normalized = {
                "evt.time": evt_time,
                "evt.type": evt_type,
                "proc.cmdline": proc_cmdline,
                "proc.pid": proc_pid,
                "proc.ppid": proc_ppid,
                "proc.pcmdline": proc_pcmdline,
                "fd.name": fd_name,
                "host": host,
                "is_warn": "true" if is_warn else "false"
            }
            
            return normalized
            
        except Exception as e:
            logger.error(f"事件规范化失败: {e}", exc_info=True)
            return {}


class EventToAlertsProcessor(MapFunction):
    """事件到告警处理器 - 生成告警并准备推理数据"""
    
    def __init__(self):
        self.rules_engine = get_rules_engine()
        self.event_normalizer = EventNormalizer()
        
    def map(self, value):
        try:
            event = json.loads(value)
            collector_id = event.get('collector_id', 'unknown')
            logger.info(f"🔍 处理事件: {event.get('event_type', 'unknown')} from {collector_id[:8]}")
            
            # 第一步: 基础规则匹配
            alerts = self.rules_engine.evaluate_event(event)
            is_warn = len(alerts) > 0
            
            if is_warn:
                logger.info(f"🚨 规则引擎匹配到 {len(alerts)} 个告警")
            
            # 第二步: 生成告警（不依赖推理结果）
            if alerts:
                alert = alerts[0]
                logger.info(f"🚨 生成告警: {alert['alert']['id']} - {alert['alert']['rule']['name']}")
                return json.dumps(alert, ensure_ascii=False)
            
            return None
                
        except Exception as e:
            logger.error(f"处理事件异常: {e}")
            return None


class EventToInferenceRequestProcessor(MapFunction):
    """事件转推理请求处理器 - 规范化事件并准备推理请求"""
    
    def __init__(self):
        self.rules_engine = get_rules_engine()
        self.event_normalizer = EventNormalizer()
        
        # 推理服务配置
        self.inference_threshold = float(os.getenv('INFERENCE_THRESHOLD', '0.5'))
        self.include_graph = True
        
    def map(self, value):
        try:
            event = json.loads(value)
            collector_id = event.get('collector_id', 'unknown')
            
            # 规则匹配判断是否为告警事件
            alerts = self.rules_engine.evaluate_event(event)
            is_warn = len(alerts) > 0
            
            # 事件规范化
            normalized_event = self.event_normalizer.normalize_event(event, is_warn)
            
            if normalized_event:
                # 构建推理请求
                inference_request = {
                    "collector_id": collector_id,
                    "events": [normalized_event],
                    "options": {
                        "threshold": self.inference_threshold,
                        "include_graph": self.include_graph
                    },
                    "timestamp": event.get('timestamp', ''),
                    "event_type": event.get('event_type', '')
                }
                
                logger.info(f"📤 生成推理请求: collector={collector_id[:8]}, is_warn={is_warn}")
                return json.dumps(inference_request, ensure_ascii=False)
            else:
                logger.warning(f"⚠️ 事件规范化失败，跳过推理请求")
                return None
                
        except Exception as e:
            logger.error(f"生成推理请求异常: {e}")
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
    logger.info("📊 Processing: sysarmor.events.audit → alerts + inference requests")
    
    # 环境变量配置
    kafka_servers = os.getenv('KAFKA_BOOTSTRAP_SERVERS', 'middleware-kafka:9092')
    input_topic = 'sysarmor.events.audit'
    output_alerts_topic = 'sysarmor.alerts.audit'  # 告警输出topic
    output_inference_topic = 'sysarmor.inference.requests'  # 推理请求输出topic
    kafka_group_id = 'sysarmor-audit-events-to-alerts-processor'
    
    # 推理服务配置
    inference_threshold = os.getenv('INFERENCE_THRESHOLD', '0.5')
    
    logger.info(f"📡 Kafka Servers: {kafka_servers}")
    logger.info(f"📥 Input Topic: {input_topic}")
    logger.info(f"📤 Output Alerts Topic: {output_alerts_topic}")
    logger.info(f"📤 Output Inference Topic: {output_inference_topic}")
    logger.info(f"👥 Consumer Group: {kafka_group_id}")
    logger.info(f"🎯 Inference Threshold: {inference_threshold}")
    logger.info("")
    
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
        
        # 创建 Kafka Producer (告警流)
        producer_props = {
            'bootstrap.servers': kafka_servers,
            'transaction.timeout.ms': '900000',
            'batch.size': '16384',
            'linger.ms': '5',
            'compression.type': 'snappy'
        }
        
        kafka_alerts_producer = FlinkKafkaProducer(
            topic=output_alerts_topic,
            serialization_schema=SimpleStringSchema(),
            producer_config=producer_props
        )
        
        # 创建 Kafka Producer (推理请求流)
        kafka_inference_producer = FlinkKafkaProducer(
            topic=output_inference_topic,
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
        
        # 威胁检测处理 - 生成告警
        alerts_stream = events_stream.map(
            EventToAlertsProcessor(),
            output_type=Types.STRING()
        ).filter(lambda x: x is not None)
        
        # 推理请求处理 - 生成推理请求
        inference_stream = events_stream.map(
            EventToInferenceRequestProcessor(),
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
        
        # 告警输出到 Kafka
        alerts_stream.add_sink(kafka_alerts_producer)
        logger.info(f"✅ 告警将写入 Kafka Topic: {output_alerts_topic}")
        
        # 推理请求输出到 Kafka
        inference_stream.add_sink(kafka_inference_producer)
        logger.info(f"✅ 推理请求将写入 Kafka Topic: {output_inference_topic}")
        
        # 所有告警写入 OpenSearch (使用 HTTP 方式)
        alerts_stream.map(opensearch_http_sink, output_type=Types.STRING())
        logger.info("✅ 所有告警将写入 OpenSearch: sysarmor-alerts-audit")
        
        # 监控输出 - 告警
        alerts_stream.map(
            lambda x: f"🚨 Alert: {json.loads(x).get('alert', {}).get('severity', 'unknown')} - {json.loads(x).get('alert', {}).get('rule', {}).get('name', 'unknown')} from {json.loads(x).get('metadata', {}).get('collector_id', 'unknown')[:8]}",
            output_type=Types.STRING()
        ).print()
        
        # 监控输出 - 推理请求
        inference_stream.map(
            lambda x: f"📦 Inference Request: collector={json.loads(x).get('collector_id', 'unknown')[:8]}, events={len(json.loads(x).get('events', []))}",
            output_type=Types.STRING()
        ).print()
        
        logger.info("🔄 Falco-style threat detection pipeline created:")
        logger.info(f"   {input_topic} -> Rule Engine -> Alerts -> {output_alerts_topic}")
        logger.info(f"   {input_topic} -> Normalizer -> Inference Requests -> {output_inference_topic}")
        
        # 显示加载的规则（使用全局单例）
        rules_engine = get_rules_engine()
        logger.info(f"🛡️ 威胁检测规则: {len(rules_engine.rules)} 个规则已加载")
        
        logger.info("🎯 数据输出:")
        logger.info(f"   - 告警 Kafka Topic: {output_alerts_topic}")
        logger.info(f"   - 推理请求 Kafka Topic: {output_inference_topic}")
        logger.info(f"   - OpenSearch索引: sysarmor-alerts-audit")
        
        # 执行作业
        logger.info("✅ Starting audit threat detection job...")
        
        job_client = env.execute_async("SysArmor-Audit-Events-to-Alerts-Processor")
        
        logger.info(f"🎯 Audit Events to Alerts job submitted successfully!")
        logger.info(f"📋 Job submitted with async execution")
        logger.info(f"🌐 Monitor at: http://localhost:8081")
        logger.info(f"📊 Processing:")
        logger.info(f"   - {input_topic} → {output_alerts_topic} (Alerts)")
        logger.info(f"   - {input_topic} → {output_inference_topic} (Inference Requests)")
        logger.info(f"🔍 View logs: docker logs -f sysarmor-flink-taskmanager-1")
        
        return "async-job-submitted"
        
    except Exception as e:
        logger.error(f"❌ Events to Alerts job failed: {e}")
        raise


if __name__ == "__main__":
    main()
