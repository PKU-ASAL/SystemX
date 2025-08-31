#!/usr/bin/env python3
"""
Kafka Raw Message Collector
Kafka 原始消息收集器 - 订阅 sysarmor-agentless-* topics 并保存原始消息
"""

import sys
import os
import json
import time
import argparse
from datetime import datetime
from kafka import KafkaConsumer
from typing import List, Dict, Optional

# 添加项目路径
sys.path.append(os.path.join(os.path.dirname(__file__), '..'))


class KafkaRawCollector:
    """Kafka 原始消息收集器"""
    
    def __init__(self, 
                 bootstrap_servers: List[str] = None,
                 max_messages: Optional[int] = None,
                 timeout_seconds: Optional[int] = None,
                 output_file: str = None):
        """
        初始化收集器
        
        Args:
            bootstrap_servers: Kafka 服务器列表
            max_messages: 最大消息数量 (与 timeout_seconds 互斥)
            timeout_seconds: 超时时间（秒）(与 max_messages 互斥)
            output_file: 输出文件路径
        """
        self.bootstrap_servers = bootstrap_servers or ['101.42.117.44:9093']
        self.max_messages = max_messages
        self.timeout_seconds = timeout_seconds
        self.output_file = output_file or self._generate_output_filename()
        self.collected_count = 0
        
        # 验证参数互斥性
        if max_messages is not None and timeout_seconds is not None:
            raise ValueError("max_messages 和 timeout_seconds 参数不能同时指定")
        
        # 设置默认值
        if max_messages is None and timeout_seconds is None:
            self.max_messages = 100  # 默认收集 100 条消息
        
    def _generate_output_filename(self) -> str:
        """生成输出文件名"""
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        # 确保 samples 目录存在
        samples_dir = "samples"
        os.makedirs(samples_dir, exist_ok=True)
        return f"{samples_dir}/kafka_samples_{timestamp}.jsonl"
    
    def discover_agentless_topics(self) -> List[str]:
        """发现所有 sysarmor-agentless-* topics"""
        try:
            from kafka.admin import KafkaAdminClient
            admin_client = KafkaAdminClient(
                bootstrap_servers=self.bootstrap_servers,
                request_timeout_ms=10000
            )
            
            metadata = admin_client.describe_topics()
            all_topics = list(metadata.keys())
            
            # 过滤匹配的 topics
            agentless_topics = [
                topic for topic in all_topics 
                if topic.startswith('sysarmor-agentless-')
            ]
            
            admin_client.close()
            return agentless_topics
            
        except Exception as e:
            print(f"⚠️  Failed to discover topics: {e}")
            # 使用默认 topics 作为后备
            return ['sysarmor-agentless-558c01dd', 'sysarmor-agentless-7bb885a8']
    
    def collect_raw_messages(self, topics: List[str]) -> bool:
        """
        收集原始消息并保存到 jsonl 文件
        
        Args:
            topics: 要订阅的 topic 列表
            
        Returns:
            bool: 是否成功收集消息
        """
        print(f"📡 Kafka Servers: {self.bootstrap_servers}")
        print(f"📋 Topics: {topics}")
        
        # 显示收集模式
        if self.max_messages is not None:
            print(f"🎯 Collection Mode: By message count ({self.max_messages} messages)")
        elif self.timeout_seconds is not None:
            print(f"⏰ Collection Mode: By time duration ({self.timeout_seconds}s)")
        
        print(f"💾 Output File: {self.output_file}")
        print("-" * 60)
        
        try:
            # 设置消费者超时时间
            consumer_timeout = None
            if self.timeout_seconds is not None:
                consumer_timeout = self.timeout_seconds * 1000
            else:
                # 如果按消息数量收集，设置一个较长的超时时间防止无限等待
                consumer_timeout = 300 * 1000  # 5 分钟
            
            # 创建 Kafka 消费者
            consumer = KafkaConsumer(
                *topics,
                bootstrap_servers=self.bootstrap_servers,
                group_id=f'raw-collector-{int(time.time())}',
                auto_offset_reset='latest',  # 从最新消息开始
                enable_auto_commit=True,
                value_deserializer=lambda m: m.decode('utf-8') if m else None,
                consumer_timeout_ms=consumer_timeout,
                max_poll_records=100
            )
            
            print("✅ Kafka consumer created successfully")
            print("🔍 Collecting raw messages...")
            
            # 确保输出目录存在
            os.makedirs(os.path.dirname(self.output_file) if os.path.dirname(self.output_file) else '.', exist_ok=True)
            
            start_time = time.time()
            
            with open(self.output_file, 'w', encoding='utf-8') as f:
                for message in consumer:
                    # 检查是否达到最大消息数（如果设置了）
                    if self.max_messages is not None and self.collected_count >= self.max_messages:
                        print(f"\n🎯 Reached max messages limit: {self.max_messages}")
                        break
                    
                    # 检查是否超时（如果设置了）
                    if self.timeout_seconds is not None:
                        elapsed = time.time() - start_time
                        if elapsed >= self.timeout_seconds:
                            print(f"\n⏰ Timeout reached: {self.timeout_seconds}s")
                            break
                    
                    if message.value:
                        # 构建原始消息记录
                        raw_record = {
                            'topic': message.topic,
                            'partition': message.partition,
                            'offset': message.offset,
                            'key': message.key.decode('utf-8') if message.key else None,
                            'timestamp': message.timestamp,
                            'timestamp_type': message.timestamp_type,
                            'value': message.value,  # 保持原始字符串格式
                            'collected_at': datetime.now().isoformat()
                        }
                        
                        # 写入 jsonl 格式（每行一个 JSON 对象）
                        f.write(json.dumps(raw_record, ensure_ascii=False) + '\n')
                        f.flush()  # 立即刷新到文件
                        
                        self.collected_count += 1
                        
                        # 显示进度
                        if self.collected_count % 10 == 0 or self.collected_count <= 10:
                            print(f"📨 Collected {self.collected_count} messages from {message.topic}")
                        elif self.collected_count % 100 == 0:
                            print(f"📨 Collected {self.collected_count} messages...")
            
            consumer.close()
            
            # 显示收集结果
            elapsed_time = time.time() - start_time
            print(f"\n📊 Collection Summary:")
            print(f"   Total Messages: {self.collected_count}")
            print(f"   Duration: {elapsed_time:.1f}s")
            print(f"   Rate: {self.collected_count/elapsed_time:.1f} msg/s" if elapsed_time > 0 else "   Rate: N/A")
            print(f"   Output File: {self.output_file}")
            
            return self.collected_count > 0
            
        except Exception as e:
            print(f"❌ Error collecting messages: {e}")
            import traceback
            traceback.print_exc()
            return False
    
    def run(self) -> bool:
        """运行收集器"""
        print("🚀 SysArmor Kafka Raw Message Collector")
        print("=" * 60)
        
        try:
            # 发现 agentless topics
            topics = self.discover_agentless_topics()
            if not topics:
                print("❌ No sysarmor-agentless-* topics found")
                return False
            
            print(f"📋 Discovered {len(topics)} agentless topics:")
            for topic in topics:
                print(f"   - {topic}")
            print()
            
            # 收集原始消息
            success = self.collect_raw_messages(topics)
            
            if success:
                print(f"\n✅ Collection completed successfully!")
                print(f"   File: {self.output_file}")
                print(f"   Messages: {self.collected_count}")
            else:
                print(f"\n❌ Collection failed or no messages collected")
            
            return success
            
        except Exception as e:
            print(f"❌ Collection failed: {e}")
            import traceback
            traceback.print_exc()
            return False


def main():
    """主函数"""
    parser = argparse.ArgumentParser(
        description='Collect raw messages from sysarmor-agentless-* Kafka topics',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # 收集 100 条消息（默认）
  python test_collect_kafka_samples.py

  # 收集 500 条消息
  python test_collect_kafka_samples.py -n 500

  # 收集 60 秒内的所有消息
  python test_collect_kafka_samples.py -t 60

  # 指定输出文件
  python test_collect_kafka_samples.py -n 100 -o my_messages.jsonl

  # 指定 Kafka 服务器
  python test_collect_kafka_samples.py -s localhost:9092,localhost:9093 -n 200

注意: -n 和 -t 参数不能同时使用
        """
    )
    
    parser.add_argument(
        '-s', '--servers',
        default='101.42.117.44:9093',
        help='Kafka bootstrap servers (comma-separated, default: 101.42.117.44:9093)'
    )
    
    # 创建互斥参数组
    collection_group = parser.add_mutually_exclusive_group()
    
    collection_group.add_argument(
        '-n', '--max-messages',
        type=int,
        help='Maximum number of messages to collect (mutually exclusive with -t)'
    )
    
    collection_group.add_argument(
        '-t', '--timeout',
        type=int,
        help='Collection timeout in seconds (mutually exclusive with -n)'
    )
    
    parser.add_argument(
        '-o', '--output',
        help='Output file path (default: samples/kafka_samples_TIMESTAMP.jsonl)'
    )
    
    parser.add_argument(
        '-v', '--verbose',
        action='store_true',
        help='Enable verbose output'
    )
    
    args = parser.parse_args()
    
    # 验证参数
    if args.max_messages is None and args.timeout is None:
        # 如果都没有指定，默认收集 100 条消息
        max_messages = 100
        timeout_seconds = None
        print("📋 Using default: collect 100 messages")
    else:
        max_messages = args.max_messages
        timeout_seconds = args.timeout
    
    # 解析服务器列表
    bootstrap_servers = [s.strip() for s in args.servers.split(',')]
    
    try:
        # 创建收集器
        collector = KafkaRawCollector(
            bootstrap_servers=bootstrap_servers,
            max_messages=max_messages,
            timeout_seconds=timeout_seconds,
            output_file=args.output
        )
        
        # 运行收集
        success = collector.run()
        
        return 0 if success else 1
        
    except ValueError as e:
        print(f"❌ Parameter error: {e}")
        return 1
    except Exception as e:
        print(f"❌ Unexpected error: {e}")
        import traceback
        traceback.print_exc()
        return 1


if __name__ == '__main__':
    exit(main())
