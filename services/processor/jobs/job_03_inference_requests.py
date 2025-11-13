#!/usr/bin/env python3
"""
SysArmor Processor - Inference Requests HTTP Sink Job
消费 sysarmor.inference.requests topic，异步发送推理请求到推理服务API
使用 Fire-and-forget 模式，不等待推理结果响应
"""

import os
import json
import logging
import requests
from typing import Dict, Any
from pyflink.datastream import StreamExecutionEnvironment, CheckpointingMode
from pyflink.datastream.connectors.kafka import FlinkKafkaConsumer
from pyflink.common.serialization import SimpleStringSchema
from pyflink.common.typeinfo import Types
from pyflink.datastream.functions import MapFunction

# 配置日志
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


class InferenceServiceHttpSink(MapFunction):
    """推理服务HTTP Sink - Fire-and-forget模式异步发送"""
    
    def __init__(self):
        # 推理服务配置
        self.inference_url = os.getenv('INFERENCE_SERVICE_URL', 'http://host.docker.internal:9999/predict')
        self.timeout = int(os.getenv('INFERENCE_TIMEOUT', '2'))  # 2秒超时
        self.headers = {'Content-Type': 'application/json'}
        
        logger.info(f"🤖 初始化推理服务HTTP Sink")
        logger.info(f"   URL: {self.inference_url}")
        logger.info(f"   Timeout: {self.timeout}s")
        
        # 统计计数器
        self.success_count = 0
        self.timeout_count = 0
        self.error_count = 0
        
    def map(self, value):
        """
        发送推理请求到推理服务（Fire-and-forget模式）
        """
        try:
            # 解析推理请求
            inference_request = json.loads(value)
            collector_id = inference_request.get('collector_id', 'unknown')
            events_count = len(inference_request.get('events', []))
            
            logger.info(f"📤 发送推理请求: collector={collector_id[:8]}, events={events_count}")
            
            # 发送HTTP POST请求
            try:
                response = requests.post(
                    self.inference_url,
                    json=inference_request,
                    headers=self.headers,
                    timeout=self.timeout
                )
                
                # 检查响应状态
                if response.status_code in [200, 201, 202]:
                    self.success_count += 1
                    logger.info(f"✅ 推理请求成功: collector={collector_id[:8]}, status={response.status_code}")
                else:
                    self.error_count += 1
                    logger.warning(f"⚠️ 推理服务响应异常: collector={collector_id[:8]}, status={response.status_code}")
                    
            except requests.exceptions.Timeout:
                self.timeout_count += 1
                logger.warning(f"⏱️ 推理请求超时: collector={collector_id[:8]} (数据可能已发送)")
                # 超时但数据可能已发送，不视为错误
                
            except requests.exceptions.ConnectionError as e:
                self.error_count += 1
                logger.error(f"🔌 推理服务连接失败: collector={collector_id[:8]}, error={e}")
                
            except Exception as e:
                self.error_count += 1
                logger.error(f"❌ 推理请求异常: collector={collector_id[:8]}, error={e}")
            
            # 定期输出统计信息
            if (self.success_count + self.timeout_count + self.error_count) % 100 == 0:
                logger.info(f"📊 推理请求统计: success={self.success_count}, timeout={self.timeout_count}, error={self.error_count}")
            
            return value
            
        except json.JSONDecodeError as e:
            logger.error(f"❌ 推理请求JSON解析失败: {e}")
            return value
            
        except Exception as e:
            logger.error(f"❌ 推理HTTP Sink异常: {e}")
            return value


def main():
    """主函数：创建推理请求HTTP Sink作业"""
    
    logger.info("🚀 Starting SysArmor Inference Requests HTTP Sink Job")
    logger.info("📋 Consuming inference requests and sending to ML service")
    logger.info("📊 Processing: sysarmor.inference.requests → HTTP API")
    
    # 环境变量配置
    kafka_servers = os.getenv('KAFKA_BOOTSTRAP_SERVERS', 'middleware-kafka:9092')
    input_topic = 'sysarmor.inference.requests'
    kafka_group_id = 'sysarmor-inference-requests-http-sink'
    
    # 推理服务配置
    inference_url = os.getenv('INFERENCE_SERVICE_URL', 'http://host.docker.internal:9999/predict')
    inference_timeout = os.getenv('INFERENCE_TIMEOUT', '2')
    
    logger.info(f"📡 Kafka Servers: {kafka_servers}")
    logger.info(f"📥 Input Topic: {input_topic}")
    logger.info(f"👥 Consumer Group: {kafka_group_id}")
    logger.info(f"🤖 Inference Service: {inference_url}")
    logger.info(f"⏱️ Inference Timeout: {inference_timeout}s")
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
            'auto.offset.reset': 'earliest',  # 处理所有推理请求
            'session.timeout.ms': '30000',
            'heartbeat.interval.ms': '10000',
            'max.poll.interval.ms': '300000'
        }
        
        kafka_consumer = FlinkKafkaConsumer(
            topics=[input_topic],
            deserialization_schema=SimpleStringSchema(),
            properties=consumer_props
        )
        
        logger.info("📋 Creating inference HTTP sink pipeline...")
        
        # 构建数据流处理管道
        inference_stream = env.add_source(kafka_consumer)
        
        # 发送到推理服务（Fire-and-forget模式）
        inference_stream.map(
            InferenceServiceHttpSink(),
            output_type=Types.STRING()
        )
        
        # 监控输出
        inference_stream.map(
            lambda x: f"📤 Sent inference request: collector={json.loads(x).get('collector_id', 'unknown')[:8]}, events={len(json.loads(x).get('events', []))}",
            output_type=Types.STRING()
        ).print()
        
        logger.info("🔄 Inference HTTP sink pipeline created:")
        logger.info(f"   {input_topic} → HTTP POST → {inference_url}")
        
        # 执行作业
        logger.info("✅ Starting inference HTTP sink job...")
        
        job_client = env.execute_async("SysArmor-Inference-Requests-HTTP-Sink")
        
        logger.info(f"🎯 Inference HTTP Sink job submitted successfully!")
        logger.info(f"📋 Job submitted with async execution")
        logger.info(f"🌐 Monitor at: http://localhost:8081")
        logger.info(f"📊 Processing: {input_topic} → {inference_url}")
        logger.info(f"🔍 View logs: docker logs -f sysarmor-flink-taskmanager-1")
        
        return "async-job-submitted"
        
    except Exception as e:
        logger.error(f"❌ Inference HTTP Sink job failed: {e}")
        raise


if __name__ == "__main__":
    main()
