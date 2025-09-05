-- Nova 分支集成: 添加 last_active 字段支持双向心跳机制
-- 迁移版本: 002
-- 创建时间: 2025-09-03

-- 添加 last_active 字段
ALTER TABLE collectors ADD COLUMN IF NOT EXISTS last_active TIMESTAMP;

-- 为现有记录设置合理的 last_active 值
UPDATE collectors SET last_active = updated_at WHERE last_active IS NULL;

-- 添加索引以提升查询性能
CREATE INDEX IF NOT EXISTS idx_collectors_last_active ON collectors(last_active);
CREATE INDEX IF NOT EXISTS idx_collectors_status_last_active ON collectors(status, last_active);

-- 添加注释
COMMENT ON COLUMN collectors.last_active IS '最后确认活跃时间 - 用于双向心跳机制中的主动确认';
