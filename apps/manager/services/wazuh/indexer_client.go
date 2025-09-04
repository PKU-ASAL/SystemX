package wazuh

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sysarmor/sysarmor/apps/manager/config"
	"github.com/sysarmor/sysarmor/apps/manager/models"
)

// IndexerClient Wazuh Indexer API客户端
type IndexerClient struct {
	config     *config.WazuhConfig
	httpClient *http.Client
}

// NewIndexerClient 创建新的Wazuh Indexer客户端
func NewIndexerClient(cfg *config.WazuhConfig) *IndexerClient {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !cfg.Wazuh.Indexer.TLSVerify,
		},
	}

	return &IndexerClient{
		config: cfg,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   cfg.Wazuh.Indexer.Timeout,
		},
	}
}

// makeRequest 发送HTTP请求到Indexer
func (c *IndexerClient) makeRequest(ctx context.Context, method, endpoint string, body interface{}) (*http.Response, error) {
	// 构建完整URL
	fullURL := fmt.Sprintf("%s%s", c.config.Wazuh.Indexer.URL, endpoint)

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求体失败: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置基本认证和请求头
	req.SetBasicAuth(c.config.Wazuh.Indexer.Username, c.config.Wazuh.Indexer.Password)
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	return resp, nil
}

// HealthCheck 健康检查
func (c *IndexerClient) HealthCheck(ctx context.Context) (*models.WazuhClusterHealthResponse, error) {
	resp, err := c.makeRequest(ctx, "GET", "/_cluster/health", nil)
	if err != nil {
		return nil, fmt.Errorf("Wazuh Indexer健康检查失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wazuh Indexer健康检查失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhClusterHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析健康检查响应失败: %w", err)
	}

	return &result, nil
}

// GetClusterInfo 获取集群信息
func (c *IndexerClient) GetClusterInfo(ctx context.Context) (interface{}, error) {
	resp, err := c.makeRequest(ctx, "GET", "/", nil)
	if err != nil {
		return nil, fmt.Errorf("获取集群信息失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取集群信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析集群信息响应失败: %w", err)
	}

	return result, nil
}

// SearchAlerts 搜索告警
func (c *IndexerClient) SearchAlerts(ctx context.Context, query *models.WazuhSearchQuery) (*models.WazuhSearchResponse, error) {
	// 构建索引名称
	indexName := "wazuh-alerts-*"
	if query.IndexType == "archives" {
		indexName = "wazuh-archives-*"
	}

	// 构建搜索请求体
	searchBody := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []interface{}{},
			},
		},
		"sort": []interface{}{
			map[string]interface{}{
				"@timestamp": map[string]interface{}{
					"order": "desc",
				},
			},
		},
		"from": query.From,
		"size": query.Size,
	}

	// 添加时间范围过滤
	if !query.StartTime.IsZero() || !query.EndTime.IsZero() {
		timeRange := map[string]interface{}{}
		if !query.StartTime.IsZero() {
			timeRange["gte"] = query.StartTime.Format(time.RFC3339)
		}
		if !query.EndTime.IsZero() {
			timeRange["lte"] = query.EndTime.Format(time.RFC3339)
		}

		searchBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = append(
			searchBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]interface{}),
			map[string]interface{}{
				"range": map[string]interface{}{
					"@timestamp": timeRange,
				},
			},
		)
	}

	// 添加代理ID过滤
	if query.AgentID != "" {
		searchBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = append(
			searchBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]interface{}),
			map[string]interface{}{
				"term": map[string]interface{}{
					"agent.id": query.AgentID,
				},
			},
		)
	}

	// 添加规则ID过滤
	if query.RuleID != "" {
		searchBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = append(
			searchBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]interface{}),
			map[string]interface{}{
				"term": map[string]interface{}{
					"rule.id": query.RuleID,
				},
			},
		)
	}

	// 添加规则级别过滤
	if query.RuleLevel > 0 {
		searchBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = append(
			searchBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]interface{}),
			map[string]interface{}{
				"term": map[string]interface{}{
					"rule.level": query.RuleLevel,
				},
			},
		)
	}

	// 处理查询条件
	if query.Query != nil {
		switch q := query.Query.(type) {
		case string:
			// 字符串查询 - 使用 query_string
			if q != "" {
				searchBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = append(
					searchBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]interface{}),
					map[string]interface{}{
						"query_string": map[string]interface{}{
							"query": q,
						},
					},
				)
			}
		case map[string]interface{}:
			// 复杂DSL查询 - 直接替换整个查询结构
			searchBody["query"] = q
		}
	}

	// 添加字段过滤
	if len(query.Fields) > 0 {
		searchBody["_source"] = query.Fields
	}

	// 构建请求端点
	endpoint := fmt.Sprintf("/%s/_search", indexName)

	resp, err := c.makeRequest(ctx, "POST", endpoint, searchBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("搜索告警失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析搜索响应失败: %w", err)
	}

	return &result, nil
}

// GetEventByID 根据ID获取单个事件
func (c *IndexerClient) GetEventByID(ctx context.Context, indexType, eventID string) (*models.WazuhEventResponse, error) {
	// 构建索引名称
	indexName := "wazuh-alerts-*"
	if indexType == "archives" {
		indexName = "wazuh-archives-*"
	}

	endpoint := fmt.Sprintf("/%s/_doc/%s", indexName, eventID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("事件未找到: %s", eventID)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取事件失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhEventResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析事件响应失败: %w", err)
	}

	return &result, nil
}

// GetAggregations 获取聚合统计
func (c *IndexerClient) GetAggregations(ctx context.Context, query *models.WazuhAggregationQuery) (*models.WazuhAggregationResponse, error) {
	// 构建索引名称
	indexName := "wazuh-alerts-*"
	if query.IndexType == "archives" {
		indexName = "wazuh-archives-*"
	}

	// 构建聚合请求体
	aggBody := map[string]interface{}{
		"size": 0, // 不返回具体文档，只返回聚合结果
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []interface{}{},
			},
		},
		"aggs": map[string]interface{}{},
	}

	// 添加时间范围过滤
	if !query.StartTime.IsZero() || !query.EndTime.IsZero() {
		timeRange := map[string]interface{}{}
		if !query.StartTime.IsZero() {
			timeRange["gte"] = query.StartTime.Format(time.RFC3339)
		}
		if !query.EndTime.IsZero() {
			timeRange["lte"] = query.EndTime.Format(time.RFC3339)
		}

		aggBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = append(
			aggBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]interface{}),
			map[string]interface{}{
				"range": map[string]interface{}{
					"@timestamp": timeRange,
				},
			},
		)
	}

	// 添加代理ID过滤
	if query.AgentID != "" {
		aggBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = append(
			aggBody["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]interface{}),
			map[string]interface{}{
				"term": map[string]interface{}{
					"agent.id": query.AgentID,
				},
			},
		)
	}

	// 构建聚合
	aggs := aggBody["aggs"].(map[string]interface{})

	// 按字段聚合
	if query.GroupBy != "" {
		aggs["group_by"] = map[string]interface{}{
			"terms": map[string]interface{}{
				"field": query.GroupBy,
				"size":  query.Size,
			},
		}
	}

	// 时间直方图聚合
	if query.DateHistogram != "" {
		aggs["date_histogram"] = map[string]interface{}{
			"date_histogram": map[string]interface{}{
				"field":    "@timestamp",
				"interval": query.DateHistogram,
			},
		}
	}

	// 统计聚合
	if len(query.Metrics) > 0 {
		for _, metric := range query.Metrics {
			switch metric.Type {
			case "count":
				aggs[metric.Name] = map[string]interface{}{
					"value_count": map[string]interface{}{
						"field": metric.Field,
					},
				}
			case "sum":
				aggs[metric.Name] = map[string]interface{}{
					"sum": map[string]interface{}{
						"field": metric.Field,
					},
				}
			case "avg":
				aggs[metric.Name] = map[string]interface{}{
					"avg": map[string]interface{}{
						"field": metric.Field,
					},
				}
			case "max":
				aggs[metric.Name] = map[string]interface{}{
					"max": map[string]interface{}{
						"field": metric.Field,
					},
				}
			case "min":
				aggs[metric.Name] = map[string]interface{}{
					"min": map[string]interface{}{
						"field": metric.Field,
					},
				}
			}
		}
	}

	endpoint := fmt.Sprintf("/%s/_search", indexName)

	resp, err := c.makeRequest(ctx, "POST", endpoint, aggBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取聚合统计失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhAggregationResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析聚合响应失败: %w", err)
	}

	return &result, nil
}

// GetIndices 获取索引列表
func (c *IndexerClient) GetIndices(ctx context.Context, pattern string) (*models.WazuhIndicesResponse, error) {
	endpoint := "/_cat/indices"
	if pattern != "" {
		endpoint += "/" + pattern
	}
	endpoint += "?format=json&h=index,docs.count,store.size,creation.date"

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取索引列表失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var indices []models.WazuhIndexInfo
	if err := json.NewDecoder(resp.Body).Decode(&indices); err != nil {
		return nil, fmt.Errorf("解析索引列表响应失败: %w", err)
	}

	return &models.WazuhIndicesResponse{
		Indices: indices,
	}, nil
}

// CreateIndex 创建索引
func (c *IndexerClient) CreateIndex(ctx context.Context, indexName string, settings map[string]interface{}) error {
	endpoint := "/" + indexName

	resp, err := c.makeRequest(ctx, "PUT", endpoint, settings)
	if err != nil {
		return fmt.Errorf("创建索引请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("创建索引失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DeleteIndex 删除索引
func (c *IndexerClient) DeleteIndex(ctx context.Context, indexName string) error {
	endpoint := "/" + indexName

	resp, err := c.makeRequest(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return fmt.Errorf("删除索引请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("删除索引失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetAlertsByAgent 根据Agent ID获取告警
func (c *IndexerClient) GetAlertsByAgent(ctx context.Context, agentID string, limit int) (*models.WazuhSearchResponse, error) {
	query := &models.WazuhSearchQuery{
		Size: limit,
		Query: map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"agent.id": agentID,
						},
					},
				},
			},
		},
	}

	return c.SearchAlerts(ctx, query)
}

// GetAlertsByRule 根据规则ID获取告警
func (c *IndexerClient) GetAlertsByRule(ctx context.Context, ruleID string, limit int) (*models.WazuhSearchResponse, error) {
	query := &models.WazuhSearchQuery{
		Size: limit,
		Query: map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"rule.id": ruleID,
						},
					},
				},
			},
		},
	}

	return c.SearchAlerts(ctx, query)
}

// GetAlertsByLevel 根据告警级别获取告警
func (c *IndexerClient) GetAlertsByLevel(ctx context.Context, level int, limit int) (*models.WazuhSearchResponse, error) {
	query := &models.WazuhSearchQuery{
		Size: limit,
		Query: map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"rule.level": level,
						},
					},
				},
			},
		},
	}

	return c.SearchAlerts(ctx, query)
}

// AggregateAlerts 聚合告警统计
func (c *IndexerClient) AggregateAlerts(ctx context.Context, aggType string, field string, size int) (*models.WazuhAggregationResponse, error) {
	query := &models.WazuhAggregationQuery{
		GroupBy: field,
		Size:    size,
	}

	return c.GetAggregations(ctx, query)
}

// CreateIndexTemplate 创建索引模板
func (c *IndexerClient) CreateIndexTemplate(ctx context.Context, templateName string, template map[string]interface{}) error {
	endpoint := "/_index_template/" + templateName

	resp, err := c.makeRequest(ctx, "PUT", endpoint, template)
	if err != nil {
		return fmt.Errorf("创建索引模板请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("创建索引模板失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetIndexTemplates 获取索引模板列表
func (c *IndexerClient) GetIndexTemplates(ctx context.Context, pattern string) (interface{}, error) {
	endpoint := "/_index_template"
	if pattern != "" {
		endpoint += "/" + pattern
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("获取索引模板请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取索引模板失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析索引模板响应失败: %w", err)
	}

	return result, nil
}

// BulkIndex 批量索引文档
func (c *IndexerClient) BulkIndex(ctx context.Context, operations []map[string]interface{}) error {
	if len(operations) == 0 {
		return nil
	}

	// 构建批量操作请求体
	var buffer bytes.Buffer
	for _, op := range operations {
		jsonData, err := json.Marshal(op)
		if err != nil {
			return fmt.Errorf("序列化批量操作失败: %w", err)
		}
		buffer.Write(jsonData)
		buffer.WriteByte('\n')
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.Wazuh.Indexer.URL+"/_bulk", &buffer)
	if err != nil {
		return fmt.Errorf("创建批量请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-ndjson")
	req.SetBasicAuth(c.config.Wazuh.Indexer.Username, c.config.Wazuh.Indexer.Password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("批量索引请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("批量索引失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetIndexStats 获取索引统计信息
func (c *IndexerClient) GetIndexStats(ctx context.Context, indexPattern string) (map[string]interface{}, error) {
	endpoint := "/_stats"
	if indexPattern != "" {
		endpoint = "/" + indexPattern + "/_stats"
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("获取索引统计请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取索引统计失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var stats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("解析索引统计响应失败: %w", err)
	}

	return stats, nil
}

// ExecuteQuery 执行自定义查询
func (c *IndexerClient) ExecuteQuery(ctx context.Context, index string, query map[string]interface{}) (*models.WazuhSearchResponse, error) {
	endpoint := "/" + index + "/_search"

	resp, err := c.makeRequest(ctx, "POST", endpoint, query)
	if err != nil {
		return nil, fmt.Errorf("执行查询请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("执行查询失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var searchResp models.WazuhSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("解析查询响应失败: %w", err)
	}

	return &searchResp, nil
}
