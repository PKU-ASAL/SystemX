#!/usr/bin/env python3
"""
批量 Kafka 导出工具 V2
使用 consumer group 和 offset 管理进行高效批量导出
每个 batch 使用第一条消息的时间戳命名文件
"""

import subprocess
import json
import os
import argparse
from datetime import datetime
import time
import tempfile
from dateutil import parser as date_parser

def get_container_name():
    """获取正确的 Kafka 容器名"""
    try:
        result = subprocess.run(
            ["docker", "ps", "--format", "{{.Names}}"],
            capture_output=True,
            text=True,
            timeout=10
        )
        
        if result.returncode == 0:
            containers = result.stdout.strip().split('\n')
            for container in containers:
                if 'kafka' in container.lower() and 'ui' not in container.lower():
                    return container
        return None
    except Exception:
        return None

def extract_timestamp_from_message(message_json):
    """从消息中提取时间戳并格式化为文件名"""
    try:
        data = json.loads(message_json)
        timestamp_fields = ['timestamp', 'processed_at', '@timestamp', 'time']
        
        for field in timestamp_fields:
            if field in data:
                timestamp_str = data[field]
                try:
                    if isinstance(timestamp_str, (int, float)):
                        dt = datetime.fromtimestamp(timestamp_str)
                    else:
                        dt = date_parser.parse(timestamp_str)
                    return dt.strftime('%Y%m%d_%H%M%S')
                except:
                    continue
        
        return datetime.now().strftime('%Y%m%d_%H%M%S')
    except:
        return datetime.now().strftime('%Y%m%d_%H%M%S')

def run_kafka_consumer_batch(bootstrap_servers, topic, batch_size, consumer_group, output_dir, batch_num):
    """使用 consumer group 导出一个批次的数据"""
    
    kafka_container = get_container_name()
    if not kafka_container:
        print("❌ 未找到 Kafka 容器")
        return False, 0, None
    
    print(f"🚀 开始导出批次 {batch_num}...")
    print(f"📡 服务器: {bootstrap_servers}")
    print(f"📋 Topic: {topic}")
    print(f"📊 批次大小: {batch_size:,}")
    print(f"👥 Consumer Group: {consumer_group}")
    print(f"🐳 Kafka 容器: {kafka_container}")
    
    # 构建 Docker 命令 - 使用 consumer group 自动管理 offset
    docker_cmd = [
        "docker", "exec", kafka_container,
        "kafka-console-consumer",
        "--bootstrap-server", bootstrap_servers,
        "--topic", topic,
        "--group", consumer_group,  # 使用 consumer group
        "--max-messages", str(batch_size),
        "--timeout-ms", "60000",  # 1分钟超时
        "--property", "print.offset=false",
        "--property", "print.partition=false",
        "--property", "print.timestamp=false"
    ]
    
    print(f"🔍 执行命令: {' '.join(docker_cmd)}")
    
    start_time = time.time()
    
    try:
        # 运行命令并捕获输出
        result = subprocess.run(
            docker_cmd,
            capture_output=True,
            text=True,
            timeout=180  # 3分钟总超时
        )
        
        elapsed_time = time.time() - start_time
        
        if result.returncode == 0:
            # 成功导出
            lines = result.stdout.strip().split('\n')
            
            # 过滤掉非JSON行
            json_lines = []
            for line in lines:
                line = line.strip()
                if line and line.startswith('{') and line.endswith('}'):
                    try:
                        json.loads(line)  # 验证JSON
                        json_lines.append(line)
                    except json.JSONDecodeError:
                        continue
            
            if not json_lines:
                print(f"⚠️  批次 {batch_num} 没有获取到有效数据")
                return False, 0, None
            
            # 使用第一条消息的时间戳命名文件
            first_message_timestamp = extract_timestamp_from_message(json_lines[0])
            filename = f"{topic}_batch_{batch_num:03d}_{first_message_timestamp}.jsonl"
            output_file = os.path.join(output_dir, filename)
            
            # 确保输出目录存在
            os.makedirs(output_dir, exist_ok=True)
            
            # 写入文件
            with open(output_file, 'w', encoding='utf-8') as f:
                for line in json_lines:
                    f.write(line + '\n')
            
            # 统计信息
            message_count = len(json_lines)
            file_size = os.path.getsize(output_file)
            rate = message_count / elapsed_time if elapsed_time > 0 else 0
            
            print(f"\n✅ 批次 {batch_num} 导出成功!")
            print(f"📊 消息数量: {message_count:,}")
            print(f"⏱️  耗时: {elapsed_time:.1f} 秒")
            print(f"📈 速度: {rate:.1f} msg/s")
            print(f"📦 文件大小: {file_size:,} bytes ({file_size/1024/1024:.1f} MB)")
            print(f"📁 输出文件: {output_file}")
            
            # 显示第一条和最后一条消息的时间戳
            try:
                first_data = json.loads(json_lines[0])
                last_data = json.loads(json_lines[-1])
                first_ts = first_data.get('timestamp', 'N/A')
                last_ts = last_data.get('timestamp', 'N/A')
                print(f"🕐 时间范围: {first_ts} -> {last_ts}")
            except:
                pass
            
            return True, message_count, output_file
            
        else:
            print(f"❌ 批次 {batch_num} 导出失败:")
            print(f"   返回码: {result.returncode}")
            if result.stderr:
                print(f"   错误输出: {result.stderr}")
            return False, 0, None
            
    except subprocess.TimeoutExpired:
        print(f"❌ 批次 {batch_num} 导出超时 (3分钟)")
        return False, 0, None
    except Exception as e:
        print(f"❌ 批次 {batch_num} 导出异常: {e}")
        return False, 0, None

def reset_consumer_group(bootstrap_servers, topic, consumer_group):
    """重置 consumer group 到最早位置"""
    kafka_container = get_container_name()
    if not kafka_container:
        return False
    
    print(f"🔄 重置 consumer group '{consumer_group}' 到最早位置...")
    
    # 重置 consumer group offset 到最早位置
    reset_cmd = [
        "docker", "exec", kafka_container,
        "kafka-consumer-groups",
        "--bootstrap-server", bootstrap_servers,
        "--group", consumer_group,
        "--topic", topic,
        "--reset-offsets",
        "--to-earliest",
        "--execute"
    ]
    
    try:
        result = subprocess.run(reset_cmd, capture_output=True, text=True, timeout=30)
        if result.returncode == 0:
            print(f"✅ Consumer group 重置成功")
            return True
        else:
            print(f"⚠️  Consumer group 重置失败: {result.stderr}")
            return True  # 继续执行，可能是新的 group
    except Exception as e:
        print(f"⚠️  Consumer group 重置异常: {e}")
        return True  # 继续执行

def export_in_batches(bootstrap_servers, topic, total_messages, batch_size, output_dir):
    """分批导出大量数据"""
    
    print(f"🔧 批量导出模式")
    if total_messages is None:
        print(f"📊 总消息数: 所有消息 (无限制)")
    else:
        print(f"📊 总消息数: {total_messages:,}")
    print(f"📦 批次大小: {batch_size:,}")
    print(f"📁 输出目录: {output_dir}")
    print("=" * 60)
    
    os.makedirs(output_dir, exist_ok=True)
    
    # 生成唯一的 consumer group 名称
    timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
    consumer_group = f"bulk-export-{timestamp}"
    
    # 重置 consumer group 到最早位置
    reset_consumer_group(bootstrap_servers, topic, consumer_group)
    
    total_exported = 0
    batch_num = 1
    exported_files = []
    consecutive_failures = 0
    
    while True:
        # 如果设置了总消息数限制，检查是否已达到
        if total_messages is not None and total_exported >= total_messages:
            print(f"✅ 已达到目标消息数: {total_messages:,}")
            break
            
        # 计算当前批次大小
        if total_messages is not None:
            remaining = total_messages - total_exported
            current_batch_size = min(batch_size, remaining)
        else:
            current_batch_size = batch_size
        
        print(f"\n📦 批次 {batch_num} (目标: {current_batch_size:,} 条消息)")
        
        # 导出当前批次
        success, count, output_file = run_kafka_consumer_batch(
            bootstrap_servers, topic, current_batch_size, consumer_group, output_dir, batch_num
        )
        
        if success and count > 0:
            total_exported += count
            exported_files.append(output_file)
            batch_num += 1
            consecutive_failures = 0
            
            if count < current_batch_size:
                print(f"⚠️  批次 {batch_num-1} 只导出了 {count:,} 条消息，可能已到达数据末尾")
                break
        else:
            consecutive_failures += 1
            print(f"❌ 批次 {batch_num} 导出失败 (连续失败: {consecutive_failures})")
            
            if consecutive_failures >= 3:
                print(f"❌ 连续失败 {consecutive_failures} 次，停止导出")
                break
            
            # 等待一下再重试
            print(f"⏳ 等待 5 秒后重试...")
            time.sleep(5)
    
    print(f"\n🎉 批量导出完成!")
    print(f"📊 总计导出: {total_exported:,} 条消息")
    print(f"📦 批次数量: {len(exported_files)}")
    print(f"📁 输出目录: {output_dir}")
    print(f"👥 Consumer Group: {consumer_group}")
    
    if exported_files:
        print(f"\n📋 导出文件列表:")
        total_size = 0
        for i, file_path in enumerate(exported_files, 1):
            file_size = os.path.getsize(file_path)
            total_size += file_size
            print(f"   {i}. {os.path.basename(file_path)} ({file_size:,} bytes)")
        print(f"📦 总文件大小: {total_size:,} bytes ({total_size/1024/1024:.1f} MB)")

def main():
    parser = argparse.ArgumentParser(description='批量 Kafka 导出工具 V2 (使用 Consumer Group)')
    parser.add_argument('--bootstrap-servers', 
                       default='localhost:9092',
                       help='Kafka bootstrap servers (默认: localhost:9092)')
    parser.add_argument('--topic', 
                       default='sysarmor-agentless-558c01dd',
                       help='Topic 名称 (默认: sysarmor-agentless-558c01dd)')
    parser.add_argument('--max-messages', 
                       type=int,
                       default=None,
                       help='最大导出消息数 (默认: 导出所有消息)')
    parser.add_argument('--batch-size', 
                       type=int,
                       default=1000000,
                       help='批次大小 (默认: 1,000,000)')
    parser.add_argument('--output-dir', 
                       required=True,
                       help='输出目录 (必需)')
    
    args = parser.parse_args()
    
    print("🔧 批量 Kafka 导出工具 V2")
    print("=" * 60)
    print(f"📡 Kafka 服务器: {args.bootstrap_servers}")
    print(f"📋 Topic: {args.topic}")
    if args.max_messages is None:
        print(f"📊 最大消息数: 所有消息 (无限制)")
    else:
        print(f"📊 最大消息数: {args.max_messages:,}")
    print(f"📦 批次大小: {args.batch_size:,}")
    print(f"📁 输出目录: {args.output_dir}")
    print("=" * 60)
    
    # 检查 Docker 和 Kafka 容器
    kafka_container = get_container_name()
    if not kafka_container:
        print("❌ 未找到运行中的 Kafka 容器")
        print("💡 请确保 Kafka 服务正在运行")
        return 1
    
    print(f"✅ 找到 Kafka 容器: {kafka_container}")
    
    # 批量导出
    export_in_batches(
        args.bootstrap_servers, args.topic, 
        args.max_messages, args.batch_size, args.output_dir
    )
    
    return 0

if __name__ == '__main__':
    exit(main())
