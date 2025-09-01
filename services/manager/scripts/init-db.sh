#!/bin/bash

# SysArmor 数据库自动初始化脚本
# 检查数据库是否已初始化，如果没有则执行迁移

set -e

echo "🔍 检查数据库初始化状态..."

# 数据库连接参数
DB_HOST="${POSTGRES_HOST:-manager-postgres}"
DB_PORT="${POSTGRES_PORT:-5432}"
DB_NAME="${POSTGRES_DB:-sysarmor}"
DB_USER="${POSTGRES_USER:-sysarmor}"
DB_PASSWORD="${POSTGRES_PASSWORD:-password}"

# 等待数据库就绪
echo "⏳ 等待数据库就绪..."
for i in {1..30}; do
    if PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "SELECT 1;" > /dev/null 2>&1; then
        echo "✅ 数据库连接成功"
        break
    fi
    echo "  等待数据库启动... ($i/30)"
    sleep 2
done

# 检查 collectors 表是否存在
echo "🔍 检查表结构..."
TABLE_EXISTS=$(PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'collectors');" | tr -d ' \n')

if [ "$TABLE_EXISTS" = "t" ]; then
    echo "✅ 数据库已初始化，跳过迁移"
    
    # 检查表结构版本（可选）
    RECORD_COUNT=$(PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c "SELECT COUNT(*) FROM collectors;" | tr -d ' \n')
    echo "📊 当前 collectors 表记录数: $RECORD_COUNT"
else
    echo "🚀 开始数据库初始化..."
    
    # 执行迁移脚本
    PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f /app/migrations/001_initial.sql
    
    echo "✅ 数据库初始化完成"
    
    # 验证初始化结果
    TABLE_EXISTS_AFTER=$(PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'collectors');" | tr -d ' \n')
    
    if [ "$TABLE_EXISTS_AFTER" = "t" ]; then
        echo "✅ 表结构创建成功"
        
        # 检查索引
        INDEX_COUNT=$(PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c "SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'collectors';" | tr -d ' \n')
        echo "📊 创建了 $INDEX_COUNT 个索引"
    else
        echo "❌ 表结构创建失败"
        exit 1
    fi
fi

echo "🎉 数据库初始化检查完成"
