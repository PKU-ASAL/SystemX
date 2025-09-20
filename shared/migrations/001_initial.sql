-- migrations/001_unified_initial.sql
-- SysArmor 统一初始化数据库表结构
-- 合并了原有的 001_initial.sql, 002_add_last_active.sql, 003_remove_kafka_topic.sql
-- 迁移版本: 001 (统一版本)
-- 创建时间: 2025-09-12

-- Collectors 表（核心表）
-- 注意：移除了 kafka_topic 字段，现在使用统一的 topic 架构
CREATE TABLE IF NOT EXISTS collectors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    collector_id VARCHAR(255) UNIQUE NOT NULL,
    hostname VARCHAR(255) NOT NULL,
    ip_address VARCHAR(255) NOT NULL,
    os_type VARCHAR(50) NOT NULL,
    os_version VARCHAR(100) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    worker_address VARCHAR(255) NOT NULL,
    deployment_type VARCHAR(50) NOT NULL DEFAULT 'agentless',
    
    -- 心跳相关字段
    last_heartbeat TIMESTAMP,
    last_active TIMESTAMP,  -- Nova 分支新增：最后确认活跃时间
    heartbeat_interval INTEGER DEFAULT 30,
    
    -- 元数据字段（用于分组管理、标签等）
    metadata JSONB DEFAULT '{}',
    config_version VARCHAR(50) DEFAULT 'v1.0',
    
    -- 基础时间戳
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 基础索引
CREATE INDEX IF NOT EXISTS idx_collectors_status ON collectors(status);
CREATE INDEX IF NOT EXISTS idx_collectors_collector_id ON collectors(collector_id);
CREATE INDEX IF NOT EXISTS idx_collectors_last_heartbeat ON collectors(last_heartbeat);
CREATE INDEX IF NOT EXISTS idx_collectors_last_active ON collectors(last_active);
CREATE INDEX IF NOT EXISTS idx_collectors_deployment_type ON collectors(deployment_type);
CREATE INDEX IF NOT EXISTS idx_collectors_created_at ON collectors(created_at);
CREATE INDEX IF NOT EXISTS idx_collectors_updated_at ON collectors(updated_at);
CREATE INDEX IF NOT EXISTS idx_collectors_hostname ON collectors(hostname);

-- 复合索引（用于双向心跳机制查询优化）
CREATE INDEX IF NOT EXISTS idx_collectors_status_last_active ON collectors(status, last_active);

-- JSONB 元数据索引（用于 Query Parameters 过滤）
-- GIN 索引用于高效的 JSONB 查询
CREATE INDEX IF NOT EXISTS idx_collectors_metadata_gin ON collectors USING GIN (metadata);

-- 特定字段的表达式索引（用于精确查询）
CREATE INDEX IF NOT EXISTS idx_collectors_metadata_group ON collectors((metadata->>'group'));
CREATE INDEX IF NOT EXISTS idx_collectors_metadata_environment ON collectors((metadata->>'environment'));
CREATE INDEX IF NOT EXISTS idx_collectors_metadata_owner ON collectors((metadata->>'owner'));
CREATE INDEX IF NOT EXISTS idx_collectors_metadata_region ON collectors((metadata->>'region'));
CREATE INDEX IF NOT EXISTS idx_collectors_metadata_purpose ON collectors((metadata->>'purpose'));

-- 标签数组索引（用于标签查询）
CREATE INDEX IF NOT EXISTS idx_collectors_metadata_tags ON collectors USING GIN ((metadata->'tags'));

-- 部署类型约束
ALTER TABLE collectors ADD CONSTRAINT chk_deployment_type 
    CHECK (deployment_type IN ('agentless', 'sysarmor-stack', 'wazuh-hybrid'));

-- 状态约束
ALTER TABLE collectors ADD CONSTRAINT chk_status 
    CHECK (status IN ('active', 'inactive', 'error', 'unregistered', 'offline'));

-- 字段注释
COMMENT ON COLUMN collectors.last_active IS '最后确认活跃时间 - 用于双向心跳机制中的主动确认';
COMMENT ON COLUMN collectors.last_heartbeat IS '最后心跳时间 - Collector 主动上报的心跳';
COMMENT ON COLUMN collectors.metadata IS 'JSONB 格式的元数据，支持标签、分组、环境等信息';
COMMENT ON COLUMN collectors.deployment_type IS '部署类型：agentless, sysarmor-stack, wazuh-hybrid';

-- 表注释
COMMENT ON TABLE collectors IS 'SysArmor Collector 注册表 - 统一 topic 架构版本';

-- 插入示例数据（可选，用于测试）
-- INSERT INTO collectors (collector_id, hostname, ip_address, os_type, os_version, worker_address, deployment_type, metadata)
-- VALUES 
--     ('example-collector-001', 'test-host-agentless', '192.168.1.100', 'Linux', 'Ubuntu 22.04', 'http://162.105.126.246:15140', 'agentless', '{"group": "test", "environment": "dev"}'),
--     ('example-collector-002', 'test-host-stack', '192.168.1.101', 'Linux', 'Ubuntu 22.04', 'http://162.105.126.246:15140', 'sysarmor-stack', '{"group": "test", "environment": "dev"}'),
--     ('example-collector-003', 'test-host-wazuh', '192.168.1.102', 'Linux', 'Ubuntu 22.04', 'http://162.105.126.246:15140', 'wazuh-hybrid', '{"group": "test", "environment": "dev"}');
