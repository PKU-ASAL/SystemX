#!/usr/bin/env python3
"""
SysArmor Auditd to Sysdig Converter - 启动脚本
用于启动auditd到sysdig格式转换的Flink作业
"""

import os
import sys
import logging
import argparse
import yaml
from pathlib import Path

# 添加作业路径到Python路径
sys.path.append('/opt/flink/usr_jobs')

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

def load_config(config_path: str) -> dict:
    """加载配置文件"""
    try:
        with open(config_path, 'r', encoding='utf-8') as f:
            config = yaml.safe_load(f)
        logger.info(f"✅ Loaded configuration from {config_path}")
        return config
    except Exception as e:
        logger.error(f"❌ Failed to load config from {config_path}: {e}")
        return {}

def set_environment_variables(config: dict):
    """根据配置设置环境变量"""
    try:
        # Kafka配置
        kafka_config = config.get('kafka', {})
        os.environ['KAFKA_BOOTSTRAP_SERVERS'] = kafka_config.get('bootstrap_servers', 'middleware-kafka:9092')
        os.environ['INPUT_TOPIC'] = kafka_config.get('input_topic', 'sysarmor-agentless-558c01dd')
        os.environ['OUTPUT_TOPIC'] = kafka_config.get('output_topic', 'sysarmor-sysdig-events')
        os.environ['KAFKA_GROUP_ID'] = kafka_config.get('consumer_group', 'sysarmor-auditd-converter-group')
        
        # Flink配置
        flink_config = config.get('flink', {})
        os.environ['FLINK_PARALLELISM'] = str(flink_config.get('parallelism', 2))
        os.environ['FLINK_CHECKPOINT_INTERVAL'] = str(flink_config.get('checkpoint_interval', 60000))
        
        # 处理配置
        processing_config = config.get('processing', {})
        process_tree_config = processing_config.get('process_tree', {})
        os.environ['PROCESS_TREE_TIME_WINDOW'] = str(process_tree_config.get('time_window', 60))
        os.environ['PROCESS_CACHE_SIZE'] = str(process_tree_config.get('cache_size', 10000))
        
        logger.info("✅ Environment variables set successfully")
        
        # 打印关键配置
        logger.info(f"📡 Kafka Servers: {os.environ['KAFKA_BOOTSTRAP_SERVERS']}")
        logger.info(f"📥 Input Topic: {os.environ['INPUT_TOPIC']}")
        logger.info(f"📤 Output Topic: {os.environ['OUTPUT_TOPIC']}")
        logger.info(f"👥 Consumer Group: {os.environ['KAFKA_GROUP_ID']}")
        logger.info(f"⚙️ Parallelism: {os.environ['FLINK_PARALLELISM']}")
        
    except Exception as e:
        logger.error(f"❌ Failed to set environment variables: {e}")
        raise

def validate_environment():
    """验证运行环境"""
    required_vars = [
        'KAFKA_BOOTSTRAP_SERVERS',
        'INPUT_TOPIC',
        'OUTPUT_TOPIC'
    ]
    
    missing_vars = []
    for var in required_vars:
        if not os.environ.get(var):
            missing_vars.append(var)
    
    if missing_vars:
        logger.error(f"❌ Missing required environment variables: {missing_vars}")
        return False
    
    logger.info("✅ Environment validation passed")
    return True

def start_converter_job():
    """启动转换作业"""
    try:
        logger.info("🚀 Starting Auditd to Sysdig Converter Job...")
        
        # 导入并运行作业
        from job_auditd_to_sysdig_converter import main
        
        # 启动作业
        job_id = main()
        
        logger.info(f"✅ Converter job started successfully!")
        logger.info(f"📋 Job ID: {job_id}")
        logger.info(f"🌐 Monitor at: http://localhost:8081")
        
        return job_id
        
    except Exception as e:
        logger.error(f"❌ Failed to start converter job: {e}")
        raise

def main():
    """主函数"""
    parser = argparse.ArgumentParser(description='SysArmor Auditd to Sysdig Converter')
    parser.add_argument(
        '--config', 
        type=str, 
        default='/opt/flink/configs/auditd-converter.yaml',
        help='Configuration file path'
    )
    parser.add_argument(
        '--input-topic',
        type=str,
        help='Input Kafka topic (overrides config)'
    )
    parser.add_argument(
        '--output-topic',
        type=str,
        help='Output Kafka topic (overrides config)'
    )
    parser.add_argument(
        '--parallelism',
        type=int,
        help='Flink job parallelism (overrides config)'
    )
    parser.add_argument(
        '--dry-run',
        action='store_true',
        help='Validate configuration without starting the job'
    )
    
    args = parser.parse_args()
    
    try:
        logger.info("🔧 SysArmor Auditd to Sysdig Converter Starting...")
        
        # 加载配置
        config = load_config(args.config)
        
        # 设置环境变量
        set_environment_variables(config)
        
        # 命令行参数覆盖
        if args.input_topic:
            os.environ['INPUT_TOPIC'] = args.input_topic
            logger.info(f"🔄 Input topic overridden: {args.input_topic}")
            
        if args.output_topic:
            os.environ['OUTPUT_TOPIC'] = args.output_topic
            logger.info(f"🔄 Output topic overridden: {args.output_topic}")
            
        if args.parallelism:
            os.environ['FLINK_PARALLELISM'] = str(args.parallelism)
            logger.info(f"🔄 Parallelism overridden: {args.parallelism}")
        
        # 验证环境
        if not validate_environment():
            sys.exit(1)
        
        if args.dry_run:
            logger.info("✅ Dry run completed successfully - configuration is valid")
            return
        
        # 启动转换作业
        job_id = start_converter_job()
        
        logger.info("🎯 Auditd to Sysdig Converter is now running!")
        logger.info("📊 Data flow: Auditd (Kafka) -> Sysdig Format -> Kafka")
        logger.info("🔍 Check Flink Web UI for job status and metrics")
        
    except KeyboardInterrupt:
        logger.info("🛑 Converter startup interrupted by user")
        sys.exit(0)
    except Exception as e:
        logger.error(f"❌ Failed to start converter: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
