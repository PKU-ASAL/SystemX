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
