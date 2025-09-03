// internal/storage/repository.go
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor/apps/manager/models"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, collector *models.Collector) error {
	// 序列化 metadata，如果为 nil 则使用空对象
	var metadataJSON []byte
	var err error
	
	if collector.Metadata != nil {
		metadataJSON, err = json.Marshal(collector.Metadata)
		if err != nil {
			return err
		}
	} else {
		// 如果 metadata 为 nil，使用空 JSON 对象
		metadataJSON = []byte("{}")
	}

	query := `
        INSERT INTO collectors (collector_id, hostname, ip_address, os_type, os_version, 
                              status, worker_address, kafka_topic, deployment_type, metadata, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err = r.db.ExecContext(ctx, query,
		collector.CollectorID,
		collector.Hostname,
		collector.IPAddress,
		collector.OSType,
		collector.OSVersion,
		collector.Status,
		collector.WorkerAddress,
		collector.KafkaTopic,
		collector.DeploymentType,
		metadataJSON,
		collector.CreatedAt,
		collector.UpdatedAt,
	)

	return err
}

func (r *Repository) GetByID(ctx context.Context, collectorID string) (*models.Collector, error) {
	query := `
        SELECT id, collector_id, hostname, ip_address, os_type, os_version,
               status, worker_address, kafka_topic, deployment_type, metadata, last_heartbeat, last_active, created_at, updated_at
        FROM collectors WHERE collector_id = $1`

	collector := &models.Collector{}
	var lastHeartbeat sql.NullTime
	var lastActive sql.NullTime
	var metadataJSON []byte

	err := r.db.QueryRowContext(ctx, query, collectorID).Scan(
		&collector.ID,
		&collector.CollectorID,
		&collector.Hostname,
		&collector.IPAddress,
		&collector.OSType,
		&collector.OSVersion,
		&collector.Status,
		&collector.WorkerAddress,
		&collector.KafkaTopic,
		&collector.DeploymentType,
		&metadataJSON,
		&lastHeartbeat,
		&lastActive,
		&collector.CreatedAt,
		&collector.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	// 反序列化 metadata
	if len(metadataJSON) > 0 {
		var metadata models.CollectorMetadata
		if err := json.Unmarshal(metadataJSON, &metadata); err == nil {
			collector.Metadata = &metadata
		}
	}

	if lastHeartbeat.Valid {
		collector.LastHeartbeat = &lastHeartbeat.Time
	}
	
	if lastActive.Valid {
		collector.LastActive = &lastActive.Time
	}

	return collector, nil
}

func (r *Repository) List(ctx context.Context) ([]*models.Collector, error) {
	query := `
        SELECT id, collector_id, hostname, ip_address, os_type, os_version,
               status, worker_address, kafka_topic, deployment_type, metadata, last_heartbeat, last_active, created_at, updated_at
        FROM collectors ORDER BY created_at DESC`

	return r.executeCollectorQuery(ctx, query)
}

// UpdateStatus 更新 Collector 状态
func (r *Repository) UpdateStatus(ctx context.Context, collectorID string, status string) error {
	query := `UPDATE collectors SET status = $1, updated_at = $2 WHERE collector_id = $3`
	now := time.Now()
	result, err := r.db.ExecContext(ctx, query, status, now, collectorID)
	if err != nil {
		return err
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	
	return nil
}

// Delete 删除 Collector
func (r *Repository) Delete(ctx context.Context, collectorID string) error {
	query := `DELETE FROM collectors WHERE collector_id = $1`
	result, err := r.db.ExecContext(ctx, query, collectorID)
	if err != nil {
		return err
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	
	return nil
}

// UpdateHeartbeat 心跳更新（预留接口）
func (r *Repository) UpdateHeartbeat(ctx context.Context, collectorID string) error {
	query := `UPDATE collectors SET last_heartbeat = $1, updated_at = $1 WHERE collector_id = $2`
	now := time.Now()
	_, err := r.db.ExecContext(ctx, query, now, collectorID)
	return err
}

// UpdateHeartbeatWithStatus 心跳状态更新（Nova分支新增）
func (r *Repository) UpdateHeartbeatWithStatus(ctx context.Context, collectorID string, status string) error {
	now := time.Now()
	
	if status == "active" {
		// 活跃状态: 同时更新 last_heartbeat 和 last_active
		query := `UPDATE collectors SET last_heartbeat = $1, last_active = $1, status = $2, updated_at = $1 WHERE collector_id = $3`
		_, err := r.db.ExecContext(ctx, query, now, status, collectorID)
		return err
	} else {
		// 非活跃状态: 只更新 last_heartbeat，保持 last_active 不变
		query := `UPDATE collectors SET last_heartbeat = $1, status = $2, updated_at = $1 WHERE collector_id = $3`
		_, err := r.db.ExecContext(ctx, query, now, status, collectorID)
		return err
	}
}

// GetActiveCollectors 获取活跃的 Collectors
func (r *Repository) GetActiveCollectors(ctx context.Context) ([]*models.Collector, error) {
	query := `
        SELECT id, collector_id, hostname, ip_address, os_type, os_version,
               status, worker_address, kafka_topic, deployment_type, metadata, last_heartbeat, last_active, created_at, updated_at
        FROM collectors 
        WHERE status = 'active' 
        ORDER BY created_at DESC`

	return r.executeCollectorQuery(ctx, query)
}

// UpdateMetadata 更新 Collector 元数据
func (r *Repository) UpdateMetadata(ctx context.Context, collectorID string, metadata *models.CollectorMetadata) error {
	var metadataJSON []byte
	var err error
	
	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return err
		}
	} else {
		// 如果 metadata 为 nil，使用空 JSON 对象
		metadataJSON = []byte("{}")
	}

	query := `UPDATE collectors SET metadata = $1, updated_at = $2 WHERE collector_id = $3`
	now := time.Now()
	_, err = r.db.ExecContext(ctx, query, metadataJSON, now, collectorID)
	return err
}

// GetByGroup 根据分组获取 Collectors（支持空值查询）
func (r *Repository) GetByGroup(ctx context.Context, group string) ([]*models.Collector, error) {
	// 使用 COALESCE 处理 null 值，并支持查询没有分组的 collectors
	var query string
	var args []interface{}
	
	if group == "null" || group == "" {
		// 查询没有设置分组的 collectors
		query = `
            SELECT id, collector_id, hostname, ip_address, os_type, os_version,
                   status, worker_address, kafka_topic, deployment_type, metadata, last_heartbeat, last_active, created_at, updated_at
            FROM collectors 
            WHERE COALESCE(metadata->>'group', '') = ''
            ORDER BY created_at DESC`
		args = []interface{}{}
	} else {
		// 查询指定分组的 collectors
		query = `
            SELECT id, collector_id, hostname, ip_address, os_type, os_version,
                   status, worker_address, kafka_topic, deployment_type, metadata, last_heartbeat, last_active, created_at, updated_at
            FROM collectors 
            WHERE metadata->>'group' = $1
            ORDER BY created_at DESC`
		args = []interface{}{group}
	}

	return r.executeCollectorQuery(ctx, query, args...)
}

// GetByTag 根据标签获取 Collectors（更健壮的查询）
func (r *Repository) GetByTag(ctx context.Context, tag string) ([]*models.Collector, error) {
	// 使用 JSONB 操作符 ? 检查数组中是否包含指定标签
	// 同时处理 tags 字段不存在或为 null 的情况
	query := `
        SELECT id, collector_id, hostname, ip_address, os_type, os_version,
               status, worker_address, kafka_topic, deployment_type, metadata, last_heartbeat, last_active, created_at, updated_at
        FROM collectors 
        WHERE metadata->'tags' ? $1
        ORDER BY created_at DESC`

	return r.executeCollectorQuery(ctx, query, tag)
}

// GetByEnvironment 根据环境获取 Collectors
func (r *Repository) GetByEnvironment(ctx context.Context, environment string) ([]*models.Collector, error) {
	var query string
	var args []interface{}
	
	if environment == "null" || environment == "" {
		query = `
            SELECT id, collector_id, hostname, ip_address, os_type, os_version,
                   status, worker_address, kafka_topic, deployment_type, metadata, last_heartbeat, last_active, created_at, updated_at
            FROM collectors 
            WHERE COALESCE(metadata->>'environment', '') = ''
            ORDER BY created_at DESC`
		args = []interface{}{}
	} else {
		query = `
            SELECT id, collector_id, hostname, ip_address, os_type, os_version,
                   status, worker_address, kafka_topic, deployment_type, metadata, last_heartbeat, last_active, created_at, updated_at
            FROM collectors 
            WHERE metadata->>'environment' = $1
            ORDER BY created_at DESC`
		args = []interface{}{environment}
	}

	return r.executeCollectorQuery(ctx, query, args...)
}

// GetByOwner 根据负责人获取 Collectors
func (r *Repository) GetByOwner(ctx context.Context, owner string) ([]*models.Collector, error) {
	query := `
        SELECT id, collector_id, hostname, ip_address, os_type, os_version,
               status, worker_address, kafka_topic, deployment_type, metadata, last_heartbeat, last_active, created_at, updated_at
        FROM collectors 
        WHERE metadata->>'owner' = $1
        ORDER BY created_at DESC`

	return r.executeCollectorQuery(ctx, query, owner)
}

// SearchCollectors 根据过滤条件搜索 Collectors
func (r *Repository) SearchCollectors(ctx context.Context, filters *models.CollectorFilters, pagination *models.PaginationRequest, sort *models.SortRequest) ([]*models.Collector, int, error) {
	// 构建 WHERE 条件
	var conditions []string
	var args []interface{}
	argIndex := 1

	if filters != nil {
		// 标签过滤（OR 关系）
		if len(filters.Tags) > 0 {
			var tagConditions []string
			for _, tag := range filters.Tags {
				tagConditions = append(tagConditions, fmt.Sprintf("metadata->'tags' ? $%d", argIndex))
				args = append(args, tag)
				argIndex++
			}
			conditions = append(conditions, "("+strings.Join(tagConditions, " OR ")+")")
		}

		// 其他字段过滤
		if filters.Group != "" {
			conditions = append(conditions, fmt.Sprintf("metadata->>'group' = $%d", argIndex))
			args = append(args, filters.Group)
			argIndex++
		}

		if filters.Environment != "" {
			conditions = append(conditions, fmt.Sprintf("metadata->>'environment' = $%d", argIndex))
			args = append(args, filters.Environment)
			argIndex++
		}

		if filters.Owner != "" {
			conditions = append(conditions, fmt.Sprintf("metadata->>'owner' = $%d", argIndex))
			args = append(args, filters.Owner)
			argIndex++
		}

		if filters.Status != "" {
			conditions = append(conditions, fmt.Sprintf("status = $%d", argIndex))
			args = append(args, filters.Status)
			argIndex++
		}

		if filters.Region != "" {
			conditions = append(conditions, fmt.Sprintf("metadata->>'region' = $%d", argIndex))
			args = append(args, filters.Region)
			argIndex++
		}

		if filters.Purpose != "" {
			conditions = append(conditions, fmt.Sprintf("metadata->>'purpose' = $%d", argIndex))
			args = append(args, filters.Purpose)
			argIndex++
		}
	}

	// 构建 WHERE 子句
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// 构建 ORDER BY 子句
	orderClause := "ORDER BY created_at DESC" // 默认排序
	if sort != nil && sort.Field != "" {
		validFields := map[string]bool{
			"created_at": true, "updated_at": true, "hostname": true,
			"status": true, "collector_id": true,
		}
		if validFields[sort.Field] {
			order := "DESC"
			if sort.Order == "asc" {
				order = "ASC"
			}
			orderClause = fmt.Sprintf("ORDER BY %s %s", sort.Field, order)
		}
	}

	// 先获取总数
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM collectors %s", whereClause)
	var total int
	err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 构建 LIMIT 和 OFFSET
	limitClause := ""
	if pagination != nil && pagination.Limit > 0 {
		offset := (pagination.Page - 1) * pagination.Limit
		limitClause = fmt.Sprintf("LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
		args = append(args, pagination.Limit, offset)
	}

	// 构建完整查询
	query := fmt.Sprintf(`
        SELECT id, collector_id, hostname, ip_address, os_type, os_version,
               status, worker_address, kafka_topic, deployment_type, metadata, last_heartbeat, last_active, created_at, updated_at
        FROM collectors 
        %s %s %s`, whereClause, orderClause, limitClause)

	collectors, err := r.executeCollectorQuery(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}

	return collectors, total, nil
}

// executeCollectorQuery 执行 collector 查询的通用方法（减少重复代码）
func (r *Repository) executeCollectorQuery(ctx context.Context, query string, args ...interface{}) ([]*models.Collector, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collectors []*models.Collector

	for rows.Next() {
		collector := &models.Collector{}
		var lastHeartbeat sql.NullTime
		var lastActive sql.NullTime
		var metadataJSON []byte

		err := rows.Scan(
			&collector.ID,
			&collector.CollectorID,
			&collector.Hostname,
			&collector.IPAddress,
			&collector.OSType,
			&collector.OSVersion,
			&collector.Status,
			&collector.WorkerAddress,
			&collector.KafkaTopic,
			&collector.DeploymentType,
			&metadataJSON,
			&lastHeartbeat,
			&lastActive,
			&collector.CreatedAt,
			&collector.UpdatedAt,
		)

		if err != nil {
			return nil, err
		}

		// 反序列化 metadata
		if len(metadataJSON) > 0 && string(metadataJSON) != "{}" {
			var metadata models.CollectorMetadata
			if err := json.Unmarshal(metadataJSON, &metadata); err == nil {
				collector.Metadata = &metadata
			}
		}

		if lastHeartbeat.Valid {
			collector.LastHeartbeat = &lastHeartbeat.Time
		}
		
		if lastActive.Valid {
			collector.LastActive = &lastActive.Time
		}

		collectors = append(collectors, collector)
	}

	return collectors, nil
}
