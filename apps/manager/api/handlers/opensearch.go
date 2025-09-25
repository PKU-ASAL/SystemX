package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sysarmor/sysarmor/apps/manager/services/opensearch"
)

// OpenSearchHandler OpenSearch 管理处理器
type OpenSearchHandler struct {
	opensearchService *opensearch.OpenSearchService
}

// NewOpenSearchHandler 创建 OpenSearch 管理处理器
func NewOpenSearchHandler(baseURL, username, password string) *OpenSearchHandler {
	return &OpenSearchHandler{
		opensearchService: opensearch.NewOpenSearchService(baseURL, username, password),
	}
}

// GetOpenSearchService 获取OpenSearch服务实例（用于其他handler）
func (h *OpenSearchHandler) GetOpenSearchService() *opensearch.OpenSearchService {
	return h.opensearchService
}

// GetOpenSearchHealth 获取 OpenSearch 健康状态
// @Summary 获取 OpenSearch 健康状态
// @Description 获取 OpenSearch 集群的健康状态和连接信息
// @Tags opensearch
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "健康状态正常"
// @Failure 500 {object} map[string]interface{} "健康状态异常"
// @Router /services/opensearch/health [get]
func (h *OpenSearchHandler) GetOpenSearchHealth(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 获取集群健康状态来测试连接
	health, err := h.opensearchService.GetClusterHealth(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":   false,
			"connected": false,
			"error":     "Failed to connect to OpenSearch: " + err.Error(),
		})
		return
	}

	// 检查集群状态
	connected := health.Status == "green" || health.Status == "yellow"

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"connected":    connected,
		"message":      "Successfully connected to OpenSearch",
		"cluster_info": health,
		"tested_at":    time.Now(),
	})
}

// GetClusterHealth 获取集群健康状态
// @Summary 获取 OpenSearch 集群健康状态
// @Description 获取 OpenSearch 集群的健康状态，包括节点数量、分片状态等
// @Tags opensearch
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "集群健康状态"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/opensearch/cluster/health [get]
func (h *OpenSearchHandler) GetClusterHealth(c *gin.Context) {
	ctx := c.Request.Context()
	
	health, err := h.opensearchService.GetClusterHealth(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get cluster health: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    health,
	})
}

// GetClusterStats 获取集群统计信息
// @Summary 获取 OpenSearch 集群统计信息
// @Description 获取 OpenSearch 集群的详细统计信息，包括索引、节点、存储等统计
// @Tags opensearch
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "集群统计信息"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/opensearch/cluster/stats [get]
func (h *OpenSearchHandler) GetClusterStats(c *gin.Context) {
	ctx := c.Request.Context()
	
	stats, err := h.opensearchService.GetClusterStats(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get cluster stats: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// GetIndices 获取索引列表
// @Summary 获取 OpenSearch 索引列表
// @Description 获取 OpenSearch 集群中所有索引的信息
// @Tags opensearch
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "索引列表"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/opensearch/indices [get]
func (h *OpenSearchHandler) GetIndices(c *gin.Context) {
	ctx := c.Request.Context()
	
	indices, err := h.opensearchService.GetIndices(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get indices: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    indices,
	})
}

// SearchEvents 搜索安全事件
// @Summary 搜索安全事件
// @Description 在指定索引模式中搜索安全事件
// @Tags opensearch
// @Accept json
// @Produce json
// @Param index query string false "索引模式，默认为 sysarmor-events-*"
// @Param q query string false "搜索查询字符串"
// @Param size query int false "返回结果数量，默认为10，最大100"
// @Param from query int false "结果偏移量，默认为0"
// @Success 200 {object} map[string]interface{} "搜索结果"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/opensearch/events/search [get]
func (h *OpenSearchHandler) SearchEvents(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 解析参数
	indexPattern := c.DefaultQuery("index", "sysarmor-events-*")
	queryString := c.Query("q")
	
	size := 10
	if sizeStr := c.Query("size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 100 {
			size = s
		}
	}
	
	from := 0
	if fromStr := c.Query("from"); fromStr != "" {
		if f, err := strconv.Atoi(fromStr); err == nil && f >= 0 {
			from = f
		}
	}
	
	// 构建搜索请求
	var request *opensearch.SearchRequest
	if queryString != "" {
		request = &opensearch.SearchRequest{
			Query: map[string]interface{}{
				"query_string": map[string]interface{}{
					"query": queryString,
				},
			},
			Size: size,
			From: from,
			Sort: []map[string]interface{}{
				{
					"@timestamp": map[string]string{
						"order": "desc",
					},
				},
			},
		}
	} else {
		request = &opensearch.SearchRequest{
			Query: map[string]interface{}{
				"match_all": map[string]interface{}{},
			},
			Size: size,
			From: from,
			Sort: []map[string]interface{}{
				{
					"@timestamp": map[string]string{
						"order": "desc",
					},
				},
			},
		}
	}
	
	result, err := h.opensearchService.SearchEvents(ctx, indexPattern, request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to search events: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetEventsByTimeRange 根据时间范围获取事件
// @Summary 根据时间范围获取安全事件
// @Description 获取指定时间范围内的安全事件
// @Tags opensearch
// @Accept json
// @Produce json
// @Param index query string false "索引模式，默认为 sysarmor-events-*"
// @Param from query string true "开始时间 (RFC3339 格式)"
// @Param to query string true "结束时间 (RFC3339 格式)"
// @Param size query int false "返回结果数量，默认为10，最大100"
// @Param page query int false "页码，默认为1"
// @Success 200 {object} map[string]interface{} "事件列表"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/opensearch/events/time-range [get]
func (h *OpenSearchHandler) GetEventsByTimeRange(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 解析参数
	indexPattern := c.DefaultQuery("index", "sysarmor-events-*")
	fromStr := c.Query("from")
	toStr := c.Query("to")
	
	if fromStr == "" || toStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "from and to parameters are required",
		})
		return
	}
	
	fromTime, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid from time format, expected RFC3339: " + err.Error(),
		})
		return
	}
	
	toTime, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid to time format, expected RFC3339: " + err.Error(),
		})
		return
	}
	
	size := 10
	if sizeStr := c.Query("size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 100 {
			size = s
		}
	}
	
	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	
	result, err := h.opensearchService.GetEventsByTimeRange(ctx, indexPattern, fromTime, toTime, size, page)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get events by time range: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetEventsByRiskScore 根据风险评分获取事件
// @Summary 根据风险评分获取安全事件
// @Description 获取风险评分高于指定阈值的安全事件
// @Tags opensearch
// @Accept json
// @Produce json
// @Param index query string false "索引模式，默认为 sysarmor-events-*"
// @Param min_score query int true "最小风险评分"
// @Param size query int false "返回结果数量，默认为10，最大100"
// @Success 200 {object} map[string]interface{} "高风险事件列表"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/opensearch/events/high-risk [get]
func (h *OpenSearchHandler) GetEventsByRiskScore(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 解析参数
	indexPattern := c.DefaultQuery("index", "sysarmor-events-*")
	minScoreStr := c.Query("min_score")
	
	if minScoreStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "min_score parameter is required",
		})
		return
	}
	
	minScore, err := strconv.Atoi(minScoreStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid min_score format: " + err.Error(),
		})
		return
	}
	
	size := 10
	if sizeStr := c.Query("size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 100 {
			size = s
		}
	}
	
	result, err := h.opensearchService.GetEventsByRiskScore(ctx, indexPattern, minScore, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get events by risk score: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetEventsBySource 根据数据源获取事件
// @Summary 根据数据源获取安全事件
// @Description 获取来自指定数据源的安全事件
// @Tags opensearch
// @Accept json
// @Produce json
// @Param index query string false "索引模式，默认为 sysarmor-events-*"
// @Param source query string true "数据源名称"
// @Param size query int false "返回结果数量，默认为10，最大100"
// @Success 200 {object} map[string]interface{} "事件列表"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/opensearch/events/by-source [get]
func (h *OpenSearchHandler) GetEventsBySource(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 解析参数
	indexPattern := c.DefaultQuery("index", "sysarmor-events-*")
	dataSource := c.Query("source")
	
	if dataSource == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "source parameter is required",
		})
		return
	}
	
	size := 10
	if sizeStr := c.Query("size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 100 {
			size = s
		}
	}
	
	result, err := h.opensearchService.GetEventsBySource(ctx, indexPattern, dataSource, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get events by source: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetThreatEvents 获取威胁事件
// @Summary 获取威胁事件
// @Description 获取标记为可疑的威胁事件
// @Tags opensearch
// @Accept json
// @Produce json
// @Param index query string false "索引模式，默认为 sysarmor-events-*"
// @Param size query int false "返回结果数量，默认为10，最大100"
// @Success 200 {object} map[string]interface{} "威胁事件列表"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/opensearch/events/threats [get]
func (h *OpenSearchHandler) GetThreatEvents(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 解析参数
	indexPattern := c.DefaultQuery("index", "sysarmor-events-*")
	
	size := 10
	if sizeStr := c.Query("size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 100 {
			size = s
		}
	}
	
	result, err := h.opensearchService.GetThreatEvents(ctx, indexPattern, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get threat events: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetEventAggregations 获取事件聚合统计
// @Summary 获取事件聚合统计
// @Description 获取安全事件的聚合统计信息，包括按数据源、严重程度、事件类型等维度的统计
// @Tags opensearch
// @Accept json
// @Produce json
// @Param index query string false "索引模式，默认为 sysarmor-events-*"
// @Success 200 {object} map[string]interface{} "聚合统计结果"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/opensearch/events/aggregations [get]
func (h *OpenSearchHandler) GetEventAggregations(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 解析参数
	indexPattern := c.DefaultQuery("index", "sysarmor-events-*")
	
	result, err := h.opensearchService.GetEventAggregations(ctx, indexPattern)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get event aggregations: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetRecentEvents 获取最近的事件
// @Summary 获取最近的安全事件
// @Description 获取最近一段时间内的安全事件
// @Tags opensearch
// @Accept json
// @Produce json
// @Param index query string false "索引模式，默认为 sysarmor-events-*"
// @Param hours query int false "时间范围（小时），默认为24小时"
// @Param size query int false "返回结果数量，默认为10，最大100"
// @Success 200 {object} map[string]interface{} "最近事件列表"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/opensearch/events/recent [get]
func (h *OpenSearchHandler) GetRecentEvents(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 解析参数
	indexPattern := c.DefaultQuery("index", "sysarmor-events-*")
	
	hours := 24
	if hoursStr := c.Query("hours"); hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 {
			hours = h
		}
	}
	
	size := 10
	if sizeStr := c.Query("size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 100 {
			size = s
		}
	}
	
	// 计算时间范围
	to := time.Now()
	from := to.Add(-time.Duration(hours) * time.Hour)
	
	result, err := h.opensearchService.GetEventsByTimeRange(ctx, indexPattern, from, to, size, 1)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get recent events: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
