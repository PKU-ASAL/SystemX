package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sysarmor/sysarmor/apps/manager/models"
	"github.com/sysarmor/sysarmor/apps/manager/services/events"
)

// EventsHandler 事件查询处理器
type EventsHandler struct {
	kafkaClient *events.KafkaClient
}

// NewEventsHandler 创建事件查询处理器
func NewEventsHandler(kafkaBrokers []string) *EventsHandler {
	return &EventsHandler{
		kafkaClient: events.NewKafkaClient(kafkaBrokers),
	}
}

// QueryEvents 查询事件
// @Summary 查询事件
// @Description 根据 Topic 和其他条件查询事件数据
// @Tags events
// @Accept json
// @Produce json
// @Param topic query string true "Kafka Topic 名称"
// @Param collector_id query string false "Collector ID 过滤"
// @Param event_type query string false "事件类型过滤"
// @Param limit query int false "返回数量限制，默认100"
// @Param latest query bool false "是否只获取最新事件"
// @Param from_time query string false "开始时间 (RFC3339格式)"
// @Param to_time query string false "结束时间 (RFC3339格式)"
// @Success 200 {object} map[string]interface{} "事件查询结果"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /events/query [get]
func (h *EventsHandler) QueryEvents(c *gin.Context) {
	ctx := c.Request.Context()

	// 解析查询参数
	params := events.QueryParams{
		Topic:       c.Query("topic"),
		CollectorID: c.Query("collector_id"),
		EventType:   c.Query("event_type"),
		Latest:      c.Query("latest") == "true",
	}

	// 解析 limit 参数
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			params.Limit = limit
		}
	}
	if params.Limit <= 0 {
		params.Limit = 100 // 默认值
	}

	// 解析时间范围参数
	if fromTimeStr := c.Query("from_time"); fromTimeStr != "" {
		if fromTime, err := time.Parse(time.RFC3339, fromTimeStr); err == nil {
			params.FromTime = fromTime
		}
	}

	if toTimeStr := c.Query("to_time"); toTimeStr != "" {
		if toTime, err := time.Parse(time.RFC3339, toTimeStr); err == nil {
			params.ToTime = toTime
		}
	}

	// 验证必需参数
	if params.Topic == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "topic parameter is required",
		})
		return
	}

	// 查询事件
	result, err := h.kafkaClient.QueryEvents(ctx, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to query events: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// QueryCollectorEvents 查询特定 collector 的事件
// @Summary 查询特定 Collector 的事件
// @Description 根据 Collector ID 查询其相关的所有事件数据
// @Tags events
// @Accept json
// @Produce json
// @Param collector_id path string true "Collector ID"
// @Param event_type query string false "事件类型过滤"
// @Param limit query int false "返回数量限制，默认100"
// @Param latest query bool false "是否只获取最新事件"
// @Param from_time query string false "开始时间 (RFC3339格式)"
// @Param to_time query string false "结束时间 (RFC3339格式)"
// @Success 200 {object} map[string]interface{} "Collector 事件查询结果"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /events/collectors/{collector_id} [get]
func (h *EventsHandler) QueryCollectorEvents(c *gin.Context) {
	ctx := c.Request.Context()
	collectorID := c.Param("collector_id")

	if collectorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "collector_id is required",
		})
		return
	}

	// 构建 topic 名称
	topic := "collector-" + collectorID

	// 解析查询参数
	params := events.QueryParams{
		Topic:       topic,
		CollectorID: collectorID,
		EventType:   c.Query("event_type"),
		Latest:      c.Query("latest") == "true",
	}

	// 解析 limit 参数
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			params.Limit = limit
		}
	}
	if params.Limit <= 0 {
		params.Limit = 100 // 默认值
	}

	// 解析时间范围参数
	if fromTimeStr := c.Query("from_time"); fromTimeStr != "" {
		if fromTime, err := time.Parse(time.RFC3339, fromTimeStr); err == nil {
			params.FromTime = fromTime
		}
	}

	if toTimeStr := c.Query("to_time"); toTimeStr != "" {
		if toTime, err := time.Parse(time.RFC3339, toTimeStr); err == nil {
			params.ToTime = toTime
		}
	}

	// 查询事件
	result, err := h.kafkaClient.QueryEvents(ctx, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to query collector events: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetLatestEvents 获取最新事件（MVP简化版本）
// @Summary 获取指定 Topic 的最新事件
// @Description 从指定 Topic 获取最新的事件数据，支持 collector_id 过滤
// @Tags events
// @Accept json
// @Produce json
// @Param topic query string false "Topic 名称，默认为 sysarmor.raw.audit.1"
// @Param collector_id query string false "Collector ID 过滤"
// @Param limit query int false "返回数量限制，默认100，最大1000"
// @Success 200 {object} map[string]interface{} "最新事件数据"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /events/latest [get]
func (h *EventsHandler) GetLatestEvents(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 获取 topic 参数，默认使用 audit topic
	topic := c.Query("topic")
	if topic == "" {
		topic = models.GetDefaultAuditTopic() // 使用常量定义的默认topic
	}
	
	// 解析查询参数
	params := events.QueryParams{
		Topic:       topic,
		CollectorID: c.Query("collector_id"),
		Latest:      true,
		Limit:       100,
	}
	
	// 解析 limit
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 1000 {
			params.Limit = limit
		}
	}
	
	// 查询最新事件
	result, err := h.kafkaClient.QueryEvents(ctx, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to query latest events: " + err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"topic":        topic,
			"collector_id": params.CollectorID,
			"events":       result.Events,
			"total":        result.Total,
			"queried_at":   time.Now(),
		},
	})
}

// GetTopicInfo 获取 Topic 信息
// GET /api/v1/events/topics/:topic/info
func (h *EventsHandler) GetTopicInfo(c *gin.Context) {
	ctx := c.Request.Context()
	topic := c.Param("topic")

	if topic == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "topic parameter is required",
		})
		return
	}

	info, err := h.kafkaClient.GetTopicInfo(ctx, topic)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get topic info: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    info,
	})
}

// ListTopics 列出所有 Topic
// @Summary 列出所有 Kafka Topics
// @Description 获取所有可用的 Kafka Topic 列表，包括 Collector 相关和其他 Topic
// @Tags events
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Topic 列表"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /events/topics [get]
func (h *EventsHandler) ListTopics(c *gin.Context) {
	ctx := c.Request.Context()

	topics, err := h.kafkaClient.ListTopics(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to list topics: " + err.Error(),
		})
		return
	}

	// 过滤 collector 相关的 topic
	var collectorTopics []string
	var otherTopics []string

	for _, topic := range topics {
		if strings.HasPrefix(topic, "collector-") {
			collectorTopics = append(collectorTopics, topic)
		} else {
			otherTopics = append(otherTopics, topic)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"collector_topics": collectorTopics,
			"other_topics":     otherTopics,
			"total_topics":     len(topics),
			"queried_at":       time.Now(),
		},
	})
}

// GetCollectorTopics 获取所有 collector 相关的 topic
// GET /api/v1/events/collectors/topics
func (h *EventsHandler) GetCollectorTopics(c *gin.Context) {
	ctx := c.Request.Context()

	topics, err := h.kafkaClient.ListTopics(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to list topics: " + err.Error(),
		})
		return
	}

	// 过滤并解析 collector topic
	var collectorTopics []gin.H
	for _, topic := range topics {
		if strings.HasPrefix(topic, "collector-") {
			collectorID := strings.TrimPrefix(topic, "collector-")
			collectorTopics = append(collectorTopics, gin.H{
				"topic":        topic,
				"collector_id": collectorID,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"collector_topics": collectorTopics,
			"total_collectors": len(collectorTopics),
			"queried_at":       time.Now(),
		},
	})
}

// SearchEvents 搜索事件
// POST /api/v1/events/search
func (h *EventsHandler) SearchEvents(c *gin.Context) {
	ctx := c.Request.Context()

	var searchRequest struct {
		Topic       string    `json:"topic" binding:"required"`
		CollectorID string    `json:"collector_id,omitempty"`
		EventType   string    `json:"event_type,omitempty"`
		Keyword     string    `json:"keyword,omitempty"`
		Limit       int       `json:"limit,omitempty"`
		FromTime    time.Time `json:"from_time,omitempty"`
		ToTime      time.Time `json:"to_time,omitempty"`
		Latest      bool      `json:"latest,omitempty"`
	}

	if err := c.ShouldBindJSON(&searchRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	// 构建查询参数
	params := events.QueryParams{
		Topic:       searchRequest.Topic,
		CollectorID: searchRequest.CollectorID,
		EventType:   searchRequest.EventType,
		Limit:       searchRequest.Limit,
		FromTime:    searchRequest.FromTime,
		ToTime:      searchRequest.ToTime,
		Latest:      searchRequest.Latest,
	}

	if params.Limit <= 0 {
		params.Limit = 100
	}

	// 查询事件
	result, err := h.kafkaClient.QueryEvents(ctx, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to search events: " + err.Error(),
		})
		return
	}

	// 如果有关键词，进行文本过滤
	if searchRequest.Keyword != "" {
		var filteredEvents []events.EventMessage
		keyword := strings.ToLower(searchRequest.Keyword)

		for _, event := range result.Events {
			if strings.Contains(strings.ToLower(event.Message), keyword) ||
				strings.Contains(strings.ToLower(event.EventType), keyword) ||
				strings.Contains(strings.ToLower(event.CollectorID), keyword) {
				filteredEvents = append(filteredEvents, event)
			}
		}

		result.Events = filteredEvents
		result.Total = len(filteredEvents)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
