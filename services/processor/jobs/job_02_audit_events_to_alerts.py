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

class ThreatDetectionRules:
    """威胁检测规则引擎 - 基于 Falco 规则设计"""
    
    def __init__(self, rules_file: str = "/opt/flink/configs/rules/threat_detection_rules.yaml"):
        self.rules = {}
        self.rule_groups = {}
        self.global_settings = {}
        self.load_rules(rules_file)
        
    def load_rules(self, rules_file: str):
        """加载威胁检测规则"""
        try:
            if os.path.exists(rules_file):
                with open(rules_file, 'r', encoding='utf-8') as f:
                    config = yaml.safe_load(f)
                
                # 加载规则
                for rule in config.get('rules', []):
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
        """加载默认规则"""
        self.rules = {
            "suspicious_tmp_execution": {
                "id": "suspicious_tmp_execution",
                "name": "可疑临时目录程序执行",
                "category": "suspicious_activity",
                "severity": "high",
                "base_score": 85,
                "patterns": [r'proc\.exe.*"/tmp/', r'proc\.exe.*"/dev/shm/'],
                "frequency_threshold": 1,
                "time_window": 300
            },
            "privilege_escalation_setuid": {
                "id": "privilege_escalation_setuid",
                "name": "SetUID权限提升",
                "category": "privilege_escalation", 
                "severity": "critical",
                "base_score": 90,
                "patterns": [r'evt\.type.*setuid', r'evt\.type.*setgid'],
                "frequency_threshold": 1,
                "time_window": 300
            }
        }
        logger.info("✅ 加载了默认威胁检测规则")
    
    def evaluate_event(self, event: Dict[str, Any]) -> List[Dict[str, Any]]:
        """评估事件是否触发威胁检测规则"""
        alerts = []
        
        # 获取事件的 sysdig 数据
        sysdig_data = event.get('message', {})
        event_str = json.dumps(event, ensure_ascii=False)
        
        for rule_id, rule in self.rules.items():
            if self._match_rule(event, sysdig_data, event_str, rule):
                alert = self._create_alert(event, rule)
                alerts.append(alert)
        
        return alerts
    
    def _match_rule(self, event: Dict, sysdig_data: Dict, event_str: str, rule: Dict) -> bool:
        """检查事件是否匹配规则"""
        try:
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
            
            # 检查字段条件匹配
            conditions = rule.get('conditions', {})
            if conditions:
                # 简单的字段匹配逻辑
                for field, expected_value in conditions.items():
                    if field in event and event[field] == expected_value:
                        return True
                    if field in sysdig_data and sysdig_data[field] == expected_value:
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
