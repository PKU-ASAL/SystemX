package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sysarmor/sysarmor/apps/manager/models"
)

// TopicsHandler Topic配置处理器
type TopicsHandler struct{}

// NewTopicsHandler 创建Topic配置处理器
func NewTopicsHandler() *TopicsHandler {
	return &TopicsHandler{}
}

// GetTopicConfigs 获取所有Topic配置
// @Summary 获取SysArmor Topic配置
// @Description 获取所有预定义的SysArmor Topic配置信息，包括分区数、保留期等
// @Tags topics
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Topic配置信息"
// @Router /topics/configs [get]
func (h *TopicsHandler) GetTopicConfigs(c *gin.Context) {
	configs := models.GetTopicConfigs()
	categories := models.GetTopicsByCategory()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"configs":    configs,
			"categories": categories,
			"total":      len(configs),
		},
	})
}

// GetTopicsByCategory 按类别获取Topics
// @Summary 按类别获取Topics
// @Description 按照数据流类别（raw、events、alerts等）获取Topic列表
// @Tags topics
// @Accept json
// @Produce json
// @Param category query string false "类别过滤 (raw, events, alerts, inference, system)"
// @Success 200 {object} map[string]interface{} "分类Topic信息"
// @Router /topics/categories [get]
func (h *TopicsHandler) GetTopicsByCategory(c *gin.Context) {
	category := c.Query("category")
	categories := models.GetTopicsByCategory()

	if category != "" {
		if topics, exists := categories[category]; exists {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"category": category,
					"topics":   topics,
					"count":    len(topics),
				},
			})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid category. Available: raw, events, alerts, inference, system",
			})
		}
		return
	}

	// 返回所有类别
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"categories": categories,
			"total":      len(categories),
		},
	})
}

// ValidateTopic 验证Topic是否有效
// @Summary 验证Topic名称
// @Description 检查指定的Topic名称是否为有效的SysArmor Topic
// @Tags topics
// @Accept json
// @Produce json
// @Param topic path string true "Topic名称"
// @Success 200 {object} map[string]interface{} "验证结果"
// @Router /topics/{topic}/validate [get]
func (h *TopicsHandler) ValidateTopic(c *gin.Context) {
	topic := c.Param("topic")

	if topic == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "topic parameter is required",
		})
		return
	}

	isValid := models.IsValidTopic(topic)
	
	response := gin.H{
		"success": true,
		"data": gin.H{
			"topic":   topic,
			"valid":   isValid,
		},
	}

	if isValid {
		configs := models.GetTopicConfigs()
		if config, exists := configs[topic]; exists {
			response["data"].(gin.H)["config"] = config
		}
	}

	c.JSON(http.StatusOK, response)
}

// GetDefaultTopics 获取默认Topics
// @Summary 获取默认Topics
// @Description 获取系统默认使用的Topic配置
// @Tags topics
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "默认Topic信息"
// @Router /topics/defaults [get]
func (h *TopicsHandler) GetDefaultTopics(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"default_audit_topic": models.GetDefaultAuditTopic(),
		"recommended_topics": gin.H{
			"raw_data": models.TopicRawAudit,
			"events":   models.TopicEventsAudit,
			"alerts":   models.TopicAlerts,
		},
		},
	})
}

// GetTopicPartitions 获取Topic分区信息
// @Summary 获取Topic分区数
// @Description 获取指定Topic的分区数配置
// @Tags topics
// @Accept json
// @Produce json
// @Param topic path string true "Topic名称"
// @Success 200 {object} map[string]interface{} "分区信息"
// @Router /topics/{topic}/partitions [get]
func (h *TopicsHandler) GetTopicPartitions(c *gin.Context) {
	topic := c.Param("topic")

	if topic == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "topic parameter is required",
		})
		return
	}

	partitions := models.GetTopicPartitions(topic)
	isValid := models.IsValidTopic(topic)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"topic":      topic,
			"partitions": partitions,
			"valid":      isValid,
		},
	})
}
