#!/usr/bin/env python3
"""
SysArmor Processor - 基于配置文件的威胁检测作业
使用 threat_detection_rules.yaml 配置文件进行威胁检测
支持动态规则加载、热重载和高级威胁检测功能
"""

import os
import json
import yaml
import re
import logging
from datetime import datetime, timedelta
from collections import defaultdict, deque
from typing import Dict, List, Any, Optional
from pyflink.datastream import StreamExecutionEnvironment, CheckpointingMode
from pyflink.datastream.connectors.kafka import FlinkKafkaConsumer
from pyflink.common.serialization import SimpleStringSchema
from pyflink.common.typeinfo import Types
from pyflink.datastream.functions import MapFunction, FilterFunction
from pyflink.common import Configuration
import requests

# 配置日志
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class ThreatRule:
    """威胁检测规则类"""
    
    def __init__(self, rule_config: Dict[str, Any]):
        self.id = rule_config.get('id')
        self.name = rule_config.get('name')
        self.description = rule_config.get('description')
        self.category = rule_config.get('category')
        self.severity = rule_config.get('severity', 'medium')
        self.base_score = rule_config.get('base_score', 50)
        self.enabled = rule_config.get('enabled', True)
        self.keywords = rule_config.get('keywords', [])
        self.patterns = rule_config.get('patterns', [])
        self.conditions = rule_config.get('conditions', {})
        self.frequency_threshold = rule_config.get('frequency_threshold', 1)
        self.time_window = rule_config.get('time_window', 300)
        self.score_multiplier = rule_config.get('score_multiplier', 1.0)
        
        # 编译正则表达式
        self.compiled_patterns = []
        for pattern in self.patterns:
            try:
                self.compiled_patterns.append(re.compile(pattern, re.IGNORECASE))
            except re.error as e:
                logger.warning(f"Invalid regex pattern in rule {self.id}: {pattern}, error: {e}")
    
    def matches_keywords(self, message: str) -> bool:
        """检查关键词匹配"""
        if not self.keywords:
            return True
        
        message_lower = message.lower()
        return any(keyword.lower() in message_lower for keyword in self.keywords)
    
    def matches_patterns(self, message: str) -> bool:
        """检查正则表达式匹配"""
        if not self.compiled_patterns:
            return True
        
        return any(pattern.search(message) for pattern in self.compiled_patterns)
    
    def matches_conditions(self, event_data: Dict[str, Any]) -> bool:
        """检查字段条件匹配"""
        if not self.conditions:
            return True
        
        for field, expected_value in self.conditions.items():
            actual_value = event_data.get(field)
            if str(actual_value) != str(expected_value):
                return False
        
        return True
    
    def matches(self, message: str, event_data: Dict[str, Any]) -> bool:
        """检查规则是否匹配"""
        if not self.enabled:
            return False
        
        return (self.matches_keywords(message) and 
                self.matches_patterns(message) and 
                self.matches_conditions(event_data))

class RuleEngine:
    """威胁规则引擎类"""
    
    def __init__(self, config_path: str = '/opt/flink/usr_jobs/../config/threat_detection_rules.yaml'):
        self.config_path = config_path
        self.rules: List[ThreatRule] = []
        self.rule_groups: Dict[str, Dict[str, Any]] = {}
        self.global_settings: Dict[str, Any] = {}
        self.metadata: Dict[str, Any] = {}
        self.frequency_tracker: Dict[str, Dict[str, deque]] = defaultdict(lambda: defaultdict(deque))
        self.last_reload_time = datetime.now()
        
        self.load_rules()
    
    def load_rules(self):
        """加载威胁检测规则"""
        try:
            if not os.path.exists(self.config_path):
                logger.warning(f"Rules config file not found: {self.config_path}, using default rules")
                self._load_default_rules()
                return
            
            with open(self.config_path, 'r', encoding='utf-8') as f:
                config = yaml.safe_load(f)
            
            # 加载元数据
            self.metadata = config.get('metadata', {})
            
            # 加载规则
            self.rules = []
            for rule_config in config.get('rules', []):
                rule = ThreatRule(rule_config)
                self.rules.append(rule)
                logger.debug(f"Loaded rule: {rule.id} - {rule.name}")
            
            # 加载规则组
            self.rule_groups = config.get('rule_groups', {})
            
            # 加载全局设置
            self.global_settings = config.get('global_settings', {})
            
            logger.info(f"✅ Loaded {len(self.rules)} threat detection rules from {self.config_path}")
            logger.info(f"📋 Rule groups: {list(self.rule_groups.keys())}")
            logger.info(f"🎯 Rules version: {self.metadata.get('version', '1.0')}")
            
        except Exception as e:
            logger.error(f"❌ Failed to load rules config: {e}")
            self._load_default_rules()
    
    def _load_default_rules(self):
        """加载默认规则（当配置文件不存在时）"""
        default_rules = [
            {
                'id': 'sudo_detection',
                'name': 'Sudo 权限提升检测',
                'category': 'privilege_escalation',
                'severity': 'high',
                'base_score': 75,
                'enabled': True,
                'keywords': ['sudo'],
                'frequency_threshold': 1,
                'score_multiplier': 1.2
            },
            {
                'id': 'suspicious_tmp_execution',
                'name': '可疑临时目录程序执行',
                'category': 'suspicious_activity',
                'severity': 'high',
                'base_score': 85,
                'enabled': True,
                'keywords': ['/tmp/', '/dev/shm/', '/var/tmp/'],
                'frequency_threshold': 1,
                'score_multiplier': 1.5
            },
            {
                'id': 'netcat_usage',
                'name': 'Netcat 使用检测',
                'category': 'command_injection',
                'severity': 'critical',
                'base_score': 90,
                'enabled': True,
                'keywords': ['netcat', 'nc -'],
                'frequency_threshold': 1,
                'score_multiplier': 1.6
            }
        ]
        
        self.rules = [ThreatRule(rule_config) for rule_config in default_rules]
        logger.info(f"✅ Loaded {len(self.rules)} default threat detection rules")
    
    def should_reload_rules(self) -> bool:
        """检查是否需要重新加载规则"""
        if not self.global_settings.get('enable_rule_reload', False):
            return False
        
        reload_interval = self.global_settings.get('rule_reload_interval', 120)
        return (datetime.now() - self.last_reload_time).seconds >= reload_interval
    
    def reload_rules_if_needed(self):
        """如果需要则重新加载规则"""
        if self.should_reload_rules():
            logger.info("🔄 Reloading threat detection rules...")
            old_count = len(self.rules)
            self.load_rules()
            new_count = len(self.rules)
            self.last_reload_time = datetime.now()
            
            if old_count != new_count:
                logger.info(f"📋 Rules updated: {old_count} -> {new_count}")
    
    def update_frequency_tracker(self, rule_id: str, host: str):
        """更新频率跟踪器"""
        now = datetime.now()
        rule = self.get_rule_by_id(rule_id)
        if not rule:
            return 0
        
        time_window = rule.time_window
        host_tracker = self.frequency_tracker[rule_id][host]
        
        # 移除过期的记录
        cutoff_time = now - timedelta(seconds=time_window)
        while host_tracker and host_tracker[0] < cutoff_time:
            host_tracker.popleft()
        
        # 添加新记录
        host_tracker.append(now)
        
        # 限制跟踪器大小
        max_tracking = self.global_settings.get('max_frequency_tracking', 2000)
        if len(host_tracker) > max_tracking:
            host_tracker.popleft()
        
        return len(host_tracker)
    
    def get_rule_by_id(self, rule_id: str) -> Optional[ThreatRule]:
        """根据ID获取规则"""
        for rule in self.rules:
            if rule.id == rule_id:
                return rule
        return None
    
    def calculate_risk_score(self, rule: ThreatRule, frequency_count: int) -> int:
        """计算风险评分"""
        # 基础评分
        risk_score = rule.base_score * rule.score_multiplier
        
        # 频率调整
        if frequency_count > rule.frequency_threshold:
            consecutive_bonus = self.global_settings.get('risk_score_adjustments', {}).get('consecutive_detection_bonus', 0.1)
            frequency_multiplier = 1 + (frequency_count - rule.frequency_threshold) * consecutive_bonus
            
            # 应用频率乘数上限
            max_multiplier = self.global_settings.get('risk_score_adjustments', {}).get('frequency_multiplier_cap', 2.0)
            frequency_multiplier = min(frequency_multiplier, max_multiplier)
            
            risk_score *= frequency_multiplier
        
        return min(100, int(risk_score))
    
    def detect_threats(self, message: str, event_data: Dict[str, Any]) -> List[Dict[str, Any]]:
        """检测威胁并返回匹配的规则"""
        # 检查是否需要重新加载规则
        self.reload_rules_if_needed()
        
        threats = []
        host = event_data.get('host', 'unknown')
        
        for rule in self.rules:
            if rule.matches(message, event_data):
                # 更新频率跟踪
                frequency_count = self.update_frequency_tracker(rule.id, host)
                
                # 检查频率阈值
                if frequency_count >= rule.frequency_threshold:
                    # 计算风险评分
                    risk_score = self.calculate_risk_score(rule, frequency_count)
                    
                    # 生成威胁ID
                    threat_id_format = self.global_settings.get('output_settings', {}).get('threat_id_format', 'SYSARMOR-{timestamp}-{rule_id}-{host}')
                    threat_id = threat_id_format.format(
                        timestamp=datetime.now().strftime('%Y%m%d%H%M%S'),
                        rule_id=rule.id,
                        host=host.replace('.', '-')
                    )
                    
                    threat = {
                        'threat_id': threat_id,
                        'rule_id': rule.id,
                        'rule_name': rule.name,
                        'category': rule.category,
                        'severity': rule.severity,
                        'risk_score': risk_score,
                        'frequency_count': frequency_count,
                        'description': rule.description,
                        'base_score': rule.base_score,
                        'score_multiplier': rule.score_multiplier
                    }
                    threats.append(threat)
                    
                    logger.info(f"🚨 Threat detected: {rule.name} (score: {risk_score}) on {host} (count: {frequency_count})")
        
        return threats
    
    def get_rule_group_info(self, rule_id: str) -> Optional[str]:
        """获取规则所属的组信息"""
        for group_name, group_info in self.rule_groups.items():
            if isinstance(group_info, dict) and 'rules' in group_info:
                if rule_id in group_info['rules']:
                    return group_name
            elif isinstance(group_info, list) and rule_id in group_info:
                return group_name
        return None

class JsonParser(MapFunction):
    """JSON 解析器"""
    
    def map(self, value):
        try:
            data = json.loads(value)
            return data
        except json.JSONDecodeError as e:
            logger.warning(f"Failed to parse JSON: {value[:100]}..., error: {e}")
            return {
                'event_type': 'unknown',
                'message': value,
                'host': 'unknown',
                'timestamp': datetime.now().isoformat()
            }
        except Exception as e:
            logger.error(f"Unexpected error parsing JSON: {e}")
            return None

class SyslogFilter(FilterFunction):
    """Syslog 事件过滤器"""
    
    def filter(self, value):
        try:
            if isinstance(value, dict):
                event_type = value.get('event_type', '')
                return event_type == 'syslog'
            return False
        except Exception as e:
            logger.error(f"Error in filter: {e}")
            return False

class ConfigurableThreatDetector(MapFunction):
    """基于配置文件的威胁检测器"""
    
    def __init__(self):
        self.rule_engine = RuleEngine()
        logger.info("🔧 Configurable threat detector initialized")
        logger.info(f"📋 Loaded rules version: {self.rule_engine.metadata.get('version', '1.0')}")
        logger.info(f"🎯 Rules description: {self.rule_engine.metadata.get('description', 'N/A')}")
    
    def map(self, value):
        try:
            if not isinstance(value, dict) or not self.rule_engine:
                return None
            
            message = value.get('message', '')
            host = value.get('host', 'unknown')
            timestamp = value.get('timestamp', datetime.now().isoformat())
            
            # 检测威胁
            threats = self.rule_engine.detect_threats(message, value)
            
            if not threats:
                return None
            
            # 选择最高风险的威胁作为主威胁
            primary_threat = max(threats, key=lambda t: t['risk_score'])
            
            # 获取规则组信息
            rule_group = self.rule_engine.get_rule_group_info(primary_threat['rule_id'])
            
            # 构建威胁事件
            threat_event = {
                '@timestamp': timestamp,
                'hostname': host,
                'message': message,
                'event_type': 'threat_detection',
                'threat_id': primary_threat['threat_id'],
                'threat_type': primary_threat['category'],
                'rule_id': primary_threat['rule_id'],
                'rule_name': primary_threat['rule_name'],
                'rule_group': rule_group,
                'risk_score': primary_threat['risk_score'],
                'base_score': primary_threat['base_score'],
                'score_multiplier': primary_threat['score_multiplier'],
                'severity': primary_threat['severity'],
                'frequency_count': primary_threat['frequency_count'],
                'description': primary_threat['description'],
                'detection_engine': 'configurable_rules',
                'detection_version': self.rule_engine.metadata.get('version', '2.0'),
                'threat_count': len(threats)
            }
            
            # 根据配置决定是否包含所有威胁和原始事件
            output_settings = self.rule_engine.global_settings.get('output_settings', {})
            
            if output_settings.get('include_rule_metadata', True):
                threat_event['all_threats'] = threats
                
            if output_settings.get('include_original_event', True):
                # 保留原始事件的其他字段
                for key in ['collector_id', 'source_type', 'program', 'port', 'processed_at', 'topic']:
                    if key in value:
                        threat_event[key] = value[key]
            
            return threat_event
            
        except Exception as e:
            logger.error(f"Error in configurable threat detection: {e}")
            return None

class PrintSink(MapFunction):
    """打印输出 Sink"""
    
    def map(self, value):
        try:
            if value:
                # 格式化输出信息
                threat_summary = {
                    'threat_id': value.get('threat_id'),
                    'rule_name': value.get('rule_name'),
                    'category': value.get('threat_type'),
                    'severity': value.get('severity'),
                    'risk_score': value.get('risk_score'),
                    'host': value.get('hostname'),
                    'frequency': value.get('frequency_count'),
                    'rule_group': value.get('rule_group')
                }
                
                print(f"🚨 CONFIG_THREAT_DETECTED: {json.dumps(threat_summary, ensure_ascii=False)}")
                logger.info(f"Configurable threat: {value.get('rule_name', 'unknown')} - Score: {value.get('risk_score', 0)} - Group: {value.get('rule_group', 'unknown')}")
            return value
        except Exception as e:
            logger.error(f"Error in print sink: {e}")
            return value

class OpenSearchSink(MapFunction):
    """OpenSearch HTTP Sink"""
    
    def __init__(self):
        self.opensearch_host = os.getenv('OPENSEARCH_HOST', 'localhost')
        self.opensearch_port = os.getenv('OPENSEARCH_PORT', '9201')
        self.opensearch_username = os.getenv('OPENSEARCH_USERNAME', 'admin')
        self.opensearch_password = os.getenv('OPENSEARCH_PASSWORD', 'admin')
        self.threats_index = os.getenv('THREATS_INDEX', 'sysarmor-threats')
        
        self.base_url = f"http://{self.opensearch_host}:{self.opensearch_port}"
        self.index_url = f"{self.base_url}/{self.threats_index}/_doc"
    
    def map(self, value):
        try:
            if not value:
                return value
            
            headers = {'Content-Type': 'application/json'}
            auth = (self.opensearch_username, self.opensearch_password)
            
            response = requests.post(
                self.index_url,
                json=value,
                headers=headers,
                auth=auth,
                timeout=10,
                verify=False
            )
            
            if response.status_code in [200, 201]:
                logger.info(f"✅ Configurable threat event written to OpenSearch: {value.get('threat_id', 'unknown')}")
            else:
                logger.error(f"❌ Failed to write configurable threat to OpenSearch: {response.status_code} - {response.text}")
                
        except requests.exceptions.RequestException as e:
            logger.error(f"❌ OpenSearch connection error: {e}")
        except Exception as e:
            logger.error(f"❌ Unexpected error in OpenSearch sink: {e}")
        
        return value

def main():
    """主函数：创建基于配置文件的威胁检测作业"""
    
    logger.info("🚀 Starting SysArmor Configurable Threat Detection Job")
    
    # 环境变量
    kafka_servers = os.getenv('KAFKA_BOOTSTRAP_SERVERS', '101.42.117.44:9093')
    kafka_group_id = os.getenv('KAFKA_GROUP_ID', 'sysarmor-configurable-group')
    
    logger.info(f"📡 Kafka Servers: {kafka_servers}")
    logger.info(f"👥 Consumer Group: {kafka_group_id}")
    
    # 创建流处理环境
    env = StreamExecutionEnvironment.get_execution_environment()
    
    # 配置环境
    env.set_parallelism(1)
    env.enable_checkpointing(60000)
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
        
        logger.info("📋 Creating configurable threat detection pipeline...")
        
        # 构建数据流处理管道
        threat_stream = env.add_source(kafka_consumer) \
            .map(JsonParser(), output_type=Types.PICKLED_BYTE_ARRAY()) \
            .filter(SyslogFilter()) \
            .map(ConfigurableThreatDetector(), output_type=Types.PICKLED_BYTE_ARRAY()) \
            .filter(lambda x: x is not None)
        
        # 分流：同时输出到控制台和 OpenSearch
        threat_stream.map(PrintSink(), output_type=Types.PICKLED_BYTE_ARRAY())
        threat_stream.map(OpenSearchSink(), output_type=Types.PICKLED_BYTE_ARRAY())
        
        logger.info("🔍 Configurable threat detection pipeline created:")
        logger.info("   Kafka Source -> JSON Parser -> Syslog Filter -> Configurable Threat Detector -> [Print Sink + OpenSearch Sink]")
        logger.info("🎯 Configurable Features:")
        logger.info("   - 基于 threat_detection_rules.yaml 配置文件")
        logger.info("   - 动态规则加载和热重载")
        logger.info("   - 频率基础威胁检测与时间窗口")
        logger.info("   - 灵活模式匹配 (关键词 + 正则 + 条件)")
        logger.info("   - 可配置风险评分和严重程度")
        logger.info("   - 规则分组和分类支持")
        
        # 执行作业
        logger.info("✅ Starting configurable threat detection job...")
        
        job_client = env.execute_async("SysArmor-Configurable-Threat-Detection")
        job_id = job_client.get_job_id()
        
        logger.info(f"🎯 Configurable threat detection job submitted successfully!")
        logger.info(f"📋 Job ID: {job_id}")
        logger.info(f"🌐 Monitor at: http://localhost:8081")
        logger.info(f"🚨 Threats will be printed with 'CONFIG_THREAT_DETECTED' prefix")
        logger.info(f"📊 Job is running with rules from threat_detection_rules.yaml")
        
        return job_id
        
    except Exception as e:
        logger.error(f"❌ Configurable threat detection job failed: {e}")
        raise

if __name__ == "__main__":
    main()
