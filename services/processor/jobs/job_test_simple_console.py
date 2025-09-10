#!/usr/bin/env python3
"""
SysArmor Processor - 简单控制台测试作业
读取指定 topic prefix 的数据并输出到控制台，用于验证 Flink 基本功能
"""

import os
import json
import logging
from datetime import datetime
from pyflink.datastream import StreamExecutionEnvironment
from pyflink.datastream.connectors.kafka import FlinkKafkaConsumer
from pyflink.common.serialization import SimpleStringSchema
from pyflink.common.typeinfo import Types
from pyflink.datastream.functions import MapFunction

# 配置日志
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class SimpleConsoleOutput(MapFunction):
    """简单的控制台输出函数"""
    
    def __init__(self):
        self.message_count = 0
        
    def map(self, value):
        try:
            self.message_count += 1
            
            # 尝试解析 JSON
            try:
                data = json.loads(value)
                timestamp = data.get('timestamp', 'N/A')
                host = data.get('host', 'N/A')
                message = data.get('message', 'N/A')
                collector_id = data.get('collector_id', 'N/A')
                
                # 格式化输出
                output = f"🔍 MESSAGE #{self.message_count} | Time: {timestamp} | Host: {host} | Collector: {collector_id[:8]}... | Content: {message[:100]}..."
                
            except json.JSONDecodeError:
                # 如果不是 JSON，直接输出原始内容
                output = f"🔍 RAW MESSAGE #{self.message_count} | Content: {value[:150]}..."
            
            # 输出到控制台
            print(output)
            logger.info(f"Processed message #{self.message_count}")
            
            return value
            
        except Exception as e:
            logger.error(f"Error processing message: {e}")
            print(f"❌ ERROR #{self.message_count}: {str(e)}")
            return value

def main():
    """主函数：创建简单的控制台测试作业"""
    
    logger.info("🚀 Starting SysArmor Simple Console Test Job")
    
    # 环境变量配置
    kafka_servers = os.getenv('KAFKA_BOOTSTRAP_SERVERS', 'localhost:9094')
    topic_prefix = os.getenv('TOPIC_PREFIX', 'sysarmor-')
    kafka_group_id = os.getenv('KAFKA_GROUP_ID', f'sysarmor-console-test-group-{datetime.now().strftime("%Y%m%d-%H%M%S")}')
    
    # 可以通过环境变量指定具体的 topics，用逗号分隔
    specific_topics = os.getenv('TEST_TOPICS', '')
    
    if specific_topics:
        topics = [topic.strip() for topic in specific_topics.split(',')]
        logger.info(f"📋 Using specific topics: {topics}")
    else:
        # 只消费测试 topic
        topics = ['sysarmor-events-test']
        logger.info(f"📋 Using test topic: {topics}")
    
    logger.info(f"📡 Kafka Servers: {kafka_servers}")
    logger.info(f"👥 Consumer Group: {kafka_group_id}")
    logger.info(f"🎯 Topic Prefix Filter: {topic_prefix}")
    
    # 创建流处理环境
    env = StreamExecutionEnvironment.get_execution_environment()
    
    # 配置环境 - 简单配置便于调试
    env.set_parallelism(1)  # 单并行度，便于观察输出顺序
    env.enable_checkpointing(30000)  # 30秒 checkpoint
    
    try:
        # 添加 Kafka JAR 依赖
        env.add_jars("file:///opt/flink/lib/flink-sql-connector-kafka-3.1.0-1.18.jar")
        
        # Kafka 连接配置
        kafka_props = {
            'bootstrap.servers': kafka_servers,
            'group.id': kafka_group_id,
            'auto.offset.reset': 'earliest',  # 从最早消息开始读取
            'session.timeout.ms': '30000',
            'heartbeat.interval.ms': '10000',
            'max.poll.interval.ms': '300000',
            'enable.auto.commit': 'true',
            'auto.commit.interval.ms': '5000'
        }
        
        # 创建 Kafka Consumer
        kafka_consumer = FlinkKafkaConsumer(
            topics=topics,
            deserialization_schema=SimpleStringSchema(),
            properties=kafka_props
        )
        
        logger.info("📋 Creating simple console test pipeline...")
        
        # 构建简单的数据流处理管道
        message_stream = env.add_source(kafka_consumer) \
            .map(SimpleConsoleOutput(), output_type=Types.STRING())
        
        logger.info("🔍 Simple test pipeline created:")
        logger.info("   Kafka Source -> Console Output")
        logger.info("🎯 Features:")
        logger.info("   - Real-time message display")
        logger.info("   - JSON parsing with fallback")
        logger.info("   - Message counter")
        logger.info("   - Error handling")
        
        logger.info("✅ Starting simple console test job...")
        logger.info("🖥️  Messages will appear in TaskManager logs")
        logger.info("📊 Monitor at: http://localhost:8081")
        logger.info("🔍 Look for '🔍 MESSAGE #' in logs")
        
        # 执行作业
        job_client = env.execute_async("SysArmor-Simple-Console-Test")
        job_id = job_client.get_job_id()
        
        logger.info(f"🎯 Simple test job submitted successfully!")
        logger.info(f"📋 Job ID: {job_id}")
        logger.info(f"🌐 Monitor at: http://localhost:8081")
        logger.info(f"📝 Check TaskManager logs for console output")
        
        return job_id
        
    except Exception as e:
        logger.error(f"❌ Simple test job failed: {e}")
        raise

if __name__ == "__main__":
    main()
