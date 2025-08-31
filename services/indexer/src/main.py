#!/usr/bin/env python3
"""
SysArmor Indexer Service
负责管理OpenSearch索引模板和索引生命周期
"""

import os
import sys
import time
import json
import logging
from typing import Dict, Any
import requests
from opensearchpy import OpenSearch, RequestsHttpConnection
import consul
import yaml

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

class SysArmorIndexer:
    """SysArmor索引管理服务"""
    
    def __init__(self):
        self.opensearch_url = os.getenv('OPENSEARCH_URL', 'http://opensearch:9200')
        self.opensearch_username = os.getenv('OPENSEARCH_USERNAME', 'admin')
        self.opensearch_password = os.getenv('OPENSEARCH_PASSWORD', 'admin')
        self.index_prefix = os.getenv('INDEX_PREFIX', 'sysarmor-events')
        self.consul_address = os.getenv('CONSUL_ADDRESS', 'consul:8500')
        
        # 初始化OpenSearch客户端
        self.opensearch_client = None
        self.consul_client = None
        
    def init_opensearch_client(self):
        """初始化OpenSearch客户端"""
        try:
            # 解析OpenSearch URL
            if self.opensearch_url.startswith('http://'):
                host = self.opensearch_url.replace('http://', '').split(':')[0]
                port = int(self.opensearch_url.replace('http://', '').split(':')[1]) if ':' in self.opensearch_url.replace('http://', '') else 9200
                use_ssl = False
            else:
                host = self.opensearch_url.replace('https://', '').split(':')[0]
                port = int(self.opensearch_url.replace('https://', '').split(':')[1]) if ':' in self.opensearch_url.replace('https://', '') else 9200
                use_ssl = True
            
            self.opensearch_client = OpenSearch(
                hosts=[{'host': host, 'port': port}],
                http_auth=(self.opensearch_username, self.opensearch_password),
                use_ssl=use_ssl,
                verify_certs=False,
                ssl_assert_hostname=False,
                ssl_show_warn=False,
                connection_class=RequestsHttpConnection
            )
            
            # 测试连接
            info = self.opensearch_client.info()
            logger.info(f"Connected to OpenSearch: {info['version']['number']}")
            return True
            
        except Exception as e:
            logger.error(f"Failed to connect to OpenSearch: {e}")
            return False
    
    def init_consul_client(self):
        """初始化Consul客户端"""
        try:
            host, port = self.consul_address.split(':')
            self.consul_client = consul.Consul(host=host, port=int(port))
            
            # 测试连接
            self.consul_client.agent.self()
            logger.info(f"Connected to Consul at {self.consul_address}")
            return True
            
        except Exception as e:
            logger.error(f"Failed to connect to Consul: {e}")
            return False
    
    def load_index_templates(self):
        """加载索引模板"""
        templates_dir = '/app/templates'
        if not os.path.exists(templates_dir):
            logger.warning(f"Templates directory not found: {templates_dir}")
            return
        
        for template_file in os.listdir(templates_dir):
            if template_file.endswith('.json'):
                template_path = os.path.join(templates_dir, template_file)
                try:
                    with open(template_path, 'r') as f:
                        template_data = json.load(f)
                    
                    template_name = template_file.replace('.json', '')
                    
                    # 创建或更新索引模板
                    response = self.opensearch_client.indices.put_template(
                        name=template_name,
                        body=template_data
                    )
                    
                    logger.info(f"Loaded index template: {template_name}")
                    
                except Exception as e:
                    logger.error(f"Failed to load template {template_file}: {e}")
    
    def create_initial_indices(self):
        """创建初始索引"""
        try:
            # 创建事件索引
            index_name = f"{self.index_prefix}-{time.strftime('%Y-%m')}"
            
            if not self.opensearch_client.indices.exists(index=index_name):
                index_settings = {
                    "settings": {
                        "number_of_shards": 1,
                        "number_of_replicas": 0,
                        "refresh_interval": "5s"
                    },
                    "mappings": {
                        "properties": {
                            "@timestamp": {"type": "date"},
                            "event_type": {"type": "keyword"},
                            "severity": {"type": "keyword"},
                            "host": {"type": "keyword"},
                            "message": {"type": "text"},
                            "raw_event": {"type": "text"}
                        }
                    }
                }
                
                self.opensearch_client.indices.create(
                    index=index_name,
                    body=index_settings
                )
                
                logger.info(f"Created index: {index_name}")
            else:
                logger.info(f"Index already exists: {index_name}")
                
        except Exception as e:
            logger.error(f"Failed to create initial indices: {e}")
    
    def register_with_consul(self):
        """向Consul注册服务"""
        try:
            service_id = "sysarmor-indexer"
            service_name = "sysarmor-indexer"
            
            # 注册服务
            self.consul_client.agent.service.register(
                name=service_name,
                service_id=service_id,
                address="indexer",
                port=8080,  # 虽然我们不暴露HTTP端口，但Consul需要一个端口
                tags=["sysarmor", "indexer", "opensearch"],
                check=consul.Check.http(
                    url=f"{self.opensearch_url}/_cluster/health",
                    interval="10s",
                    timeout="5s"
                )
            )
            
            logger.info(f"Registered service with Consul: {service_id}")
            
        except Exception as e:
            logger.error(f"Failed to register with Consul: {e}")
    
    def health_check_loop(self):
        """健康检查循环"""
        while True:
            try:
                # 检查OpenSearch连接
                health = self.opensearch_client.cluster.health()
                logger.debug(f"OpenSearch cluster health: {health['status']}")
                
                # 检查索引状态
                indices = self.opensearch_client.indices.get_alias(index=f"{self.index_prefix}-*")
                logger.debug(f"Active indices: {len(indices)}")
                
                time.sleep(30)  # 每30秒检查一次
                
            except Exception as e:
                logger.error(f"Health check failed: {e}")
                time.sleep(10)  # 出错时更频繁地重试
    
    def run(self):
        """运行索引服务"""
        logger.info("Starting SysArmor Indexer Service...")
        
        # 等待依赖服务启动
        max_retries = 30
        retry_count = 0
        
        while retry_count < max_retries:
            if self.init_opensearch_client() and self.init_consul_client():
                break
            retry_count += 1
            logger.info(f"Waiting for dependencies... ({retry_count}/{max_retries})")
            time.sleep(10)
        
        if retry_count >= max_retries:
            logger.error("Failed to connect to dependencies after maximum retries")
            sys.exit(1)
        
        # 加载索引模板
        self.load_index_templates()
        
        # 创建初始索引
        self.create_initial_indices()
        
        # 注册到Consul
        self.register_with_consul()
        
        logger.info("SysArmor Indexer Service started successfully")
        
        # 进入健康检查循环
        self.health_check_loop()

def main():
    """主函数"""
    indexer = SysArmorIndexer()
    try:
        indexer.run()
    except KeyboardInterrupt:
        logger.info("Shutting down SysArmor Indexer Service...")
    except Exception as e:
        logger.error(f"Unexpected error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
