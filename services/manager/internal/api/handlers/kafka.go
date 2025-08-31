package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sysarmor/sysarmor/services/manager/internal/services/kafka"
)

// KafkaHandler Kafka 管理处理器
type KafkaHandler struct {
	kafkaService *kafka.KafkaService
}

// NewKafkaHandler 创建 Kafka 管理处理器
func NewKafkaHandler(kafkaBrokers []string) *KafkaHandler {
	return &KafkaHandler{
		kafkaService: kafka.NewKafkaService(kafkaBrokers),
	}
}

// TestKafkaConnection 测试 Kafka 连接
// @Summary 测试 Kafka 连接
// @Description 测试与 Kafka 集群的连接状态
// @Tags kafka
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "连接成功"
// @Failure 500 {object} map[string]interface{} "连接失败"
// @Router /kafka/test-connection [get]
func (h *KafkaHandler) TestKafkaConnection(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 尝试获取集群信息来测试连接
	clusters, err := h.kafkaService.GetClusters(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":   false,
			"connected": false,
			"error":     "Failed to connect to Kafka: " + err.Error(),
		})
		return
	}

	// 获取 broker 信息
	brokers, err := h.kafkaService.GetBrokers(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":   false,
			"connected": false,
			"error":     "Failed to get broker info: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"connected":    true,
		"message":      "Successfully connected to Kafka",
		"cluster_info": clusters,
		"broker_count": len(brokers),
		"tested_at":    time.Now(),
	})
}

// GetClusters 获取 Kafka 集群信息
// @Summary 获取 Kafka 集群信息
// @Description 获取 Kafka 集群的基本信息，包括状态、Broker 数量、Topic 数量等
// @Tags kafka
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "集群信息"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/clusters [get]
func (h *KafkaHandler) GetClusters(c *gin.Context) {
	ctx := c.Request.Context()
	
	clusters, err := h.kafkaService.GetClusters(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get clusters: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    clusters,
	})
}

// GetBrokers 获取 Kafka Brokers 信息
// @Summary 获取 Kafka Brokers 信息
// @Description 获取 Kafka 集群中所有 Broker 的详细信息
// @Tags kafka
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Brokers 信息"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/brokers [get]
func (h *KafkaHandler) GetBrokers(c *gin.Context) {
	ctx := c.Request.Context()
	
	brokers, err := h.kafkaService.GetBrokers(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get brokers: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    brokers,
	})
}

// GetTopics 获取 Kafka Topics 列表（增强版）
// @Summary 获取 Kafka Topics 列表（增强版）
// @Description 获取 Kafka 集群中所有 Topic 的详细信息列表，包含 Prometheus 指标数据，支持分页和搜索
// @Tags kafka
// @Accept json
// @Produce json
// @Param page query int false "页码，默认为1"
// @Param limit query int false "每页数量，默认为20"
// @Param search query string false "搜索关键词"
// @Success 200 {object} map[string]interface{} "Topics 列表（包含详细指标）"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/topics [get]
func (h *KafkaHandler) GetTopics(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 解析分页参数
	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	
	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}
	
	search := c.Query("search")
	
	// 使用增强版方法获取包含 Prometheus 指标的 Topic 信息
	topics, err := h.kafkaService.GetTopicsListEnhanced(ctx, page, limit, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get topics: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    topics,
	})
}

// GetTopicsOverview 获取 Topics 概览信息
// @Summary 获取 Topics 概览信息
// @Description 获取 Kafka 集群中所有 Topic 的概览信息，包括分区数量、消息数量、大小、速率等统计数据
// @Tags kafka
// @Accept json
// @Produce json
// @Success 200 {object} kafka.TopicsOverviewResponse "Topics 概览信息"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/topics/overview [get]
func (h *KafkaHandler) GetTopicsOverview(c *gin.Context) {
	ctx := c.Request.Context()
	
	overview, err := h.kafkaService.GetTopicsOverview(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get topics overview: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    overview,
	})
}

// GetBrokersOverview 获取 Brokers 概览信息
// @Summary 获取 Brokers 概览信息
// @Description 获取 Kafka 集群中所有 Broker 的概览信息，包括集群状态、Broker 详情、性能指标等
// @Tags kafka
// @Accept json
// @Produce json
// @Success 200 {object} kafka.BrokersOverviewResponse "Brokers 概览信息"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/brokers/overview [get]
func (h *KafkaHandler) GetBrokersOverview(c *gin.Context) {
	ctx := c.Request.Context()
	
	overview, err := h.kafkaService.GetBrokersOverview(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get brokers overview: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    overview,
	})
}

// GetTopicDetails 获取特定 Topic 的详细信息（增强版）
// @Summary 获取特定 Topic 的详细信息（增强版）
// @Description 获取指定 Topic 的详细信息，包括分区、副本、分区大小等增强信息
// @Tags kafka
// @Accept json
// @Produce json
// @Param topic path string true "Topic 名称"
// @Success 200 {object} kafka.TopicDetailsResponse "Topic 详细信息（增强版）"
// @Failure 404 {object} map[string]interface{} "Topic 不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/topics/{topic} [get]
func (h *KafkaHandler) GetTopicDetails(c *gin.Context) {
	ctx := c.Request.Context()
	topicName := c.Param("topic")
	
	if topicName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "topic name is required",
		})
		return
	}
	
	// 使用增强版方法获取包含分区详情的 Topic 信息
	topic, err := h.kafkaService.GetTopicDetailsEnhanced(ctx, topicName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get topic details: " + err.Error(),
		})
		return
	}
	
	if topic == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Topic not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    topic,
	})
}

// GetTopicMessages 获取 Topic 消息
// @Summary 获取 Topic 消息
// @Description 获取指定 Topic 的消息内容，支持分页和过滤
// @Tags kafka
// @Accept json
// @Produce json
// @Param topic path string true "Topic 名称"
// @Param limit query int false "消息数量限制，默认为10，最大100"
// @Param partition query int false "指定分区"
// @Param offset query string false "起始偏移量，可以是数字或'earliest'/'latest'"
// @Success 200 {object} map[string]interface{} "Topic 消息"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/topics/{topic}/messages [get]
func (h *KafkaHandler) GetTopicMessages(c *gin.Context) {
	ctx := c.Request.Context()
	topicName := c.Param("topic")
	
	if topicName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "topic name is required",
		})
		return
	}
	
	// 解析参数
	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}
	
	partition := -1 // -1 表示所有分区
	if partitionStr := c.Query("partition"); partitionStr != "" {
		if p, err := strconv.Atoi(partitionStr); err == nil && p >= 0 {
			partition = p
		}
	}
	
	offset := c.Query("offset")
	if offset == "" {
		offset = "latest"
	}
	
	messages, err := h.kafkaService.GetTopicMessages(ctx, topicName, limit, partition, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get topic messages: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    messages,
	})
}

// GetConsumerGroups 获取 Consumer Groups 信息
// @Summary 获取 Consumer Groups 信息
// @Description 获取 Kafka 集群中所有 Consumer Group 的信息
// @Tags kafka
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Consumer Groups 信息"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/consumer-groups [get]
func (h *KafkaHandler) GetConsumerGroups(c *gin.Context) {
	ctx := c.Request.Context()
	
	groups, err := h.kafkaService.GetConsumerGroups(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get consumer groups: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    groups,
	})
}

// GetConsumerGroupDetails 获取特定 Consumer Group 的详细信息
// @Summary 获取特定 Consumer Group 的详细信息
// @Description 获取指定 Consumer Group 的详细信息，包括成员、偏移量等
// @Tags kafka
// @Accept json
// @Produce json
// @Param group path string true "Consumer Group 名称"
// @Success 200 {object} map[string]interface{} "Consumer Group 详细信息"
// @Failure 404 {object} map[string]interface{} "Consumer Group 不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/consumer-groups/{group} [get]
func (h *KafkaHandler) GetConsumerGroupDetails(c *gin.Context) {
	ctx := c.Request.Context()
	groupName := c.Param("group")
	
	if groupName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "consumer group name is required",
		})
		return
	}
	
	group, err := h.kafkaService.GetConsumerGroupDetails(ctx, groupName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get consumer group details: " + err.Error(),
		})
		return
	}
	
	if group == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Consumer group not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    group,
	})
}

// CreateTopic 创建新的 Topic
// @Summary 创建新的 Topic
// @Description 创建一个新的 Kafka Topic
// @Tags kafka
// @Accept json
// @Produce json
// @Param request body kafka.CreateTopicRequest true "创建 Topic 请求"
// @Success 201 {object} map[string]interface{} "Topic 创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 409 {object} map[string]interface{} "Topic 已存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/topics [post]
func (h *KafkaHandler) CreateTopic(c *gin.Context) {
	ctx := c.Request.Context()
	
	var req kafka.CreateTopicRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format: " + err.Error(),
		})
		return
	}
	
	// 验证必需字段
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "topic name is required",
		})
		return
	}
	
	err := h.kafkaService.CreateTopic(ctx, &req)
	if err != nil {
		if err == kafka.ErrTopicExists {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Topic already exists",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to create topic: " + err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Topic created successfully",
		"data": gin.H{
			"name":              req.Name,
			"partitions":        req.Partitions,
			"replication_factor": req.ReplicationFactor,
		},
	})
}

// DeleteTopic 删除 Topic
// @Summary 删除 Topic
// @Description 删除指定的 Kafka Topic
// @Tags kafka
// @Accept json
// @Produce json
// @Param topic path string true "Topic 名称"
// @Param force query bool false "强制删除，忽略错误"
// @Success 200 {object} map[string]interface{} "Topic 删除成功"
// @Failure 404 {object} map[string]interface{} "Topic 不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/topics/{topic} [delete]
func (h *KafkaHandler) DeleteTopic(c *gin.Context) {
	ctx := c.Request.Context()
	topicName := c.Param("topic")
	
	if topicName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "topic name is required",
		})
		return
	}
	
	// 检查是否强制删除
	force := false
	if forceStr := c.Query("force"); forceStr != "" {
		if f, err := strconv.ParseBool(forceStr); err == nil {
			force = f
		}
	}
	
	err := h.kafkaService.DeleteTopic(ctx, topicName, force)
	if err != nil {
		if err == kafka.ErrTopicNotFound {
			if force {
				// 强制删除模式下，即使 topic 不存在也返回成功
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": "Topic deleted successfully (was not found)",
					"data": gin.H{
						"topic": topicName,
					},
				})
				return
			}
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Topic not found",
			})
		} else {
			if force {
				// 强制删除模式下，记录错误但返回成功
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": "Topic deletion attempted (force mode)",
					"warning": "Error occurred but ignored due to force mode: " + err.Error(),
					"data": gin.H{
						"topic": topicName,
					},
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to delete topic: " + err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Topic deleted successfully",
		"data": gin.H{
			"topic": topicName,
		},
	})
}

// GetTopicConfig 获取 Topic 配置
// @Summary 获取 Topic 配置
// @Description 获取指定 Topic 的详细配置信息，包括 retention.ms、segment.ms 等所有配置参数
// @Tags kafka
// @Accept json
// @Produce json
// @Param topic path string true "Topic 名称"
// @Success 200 {object} map[string]interface{} "Topic 配置信息"
// @Failure 404 {object} map[string]interface{} "Topic 不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/topics/{topic}/config [get]
func (h *KafkaHandler) GetTopicConfig(c *gin.Context) {
	ctx := c.Request.Context()
	topicName := c.Param("topic")
	
	if topicName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "topic name is required",
		})
		return
	}
	
	config, err := h.kafkaService.GetTopicConfig(ctx, topicName)
	if err != nil {
		if err == kafka.ErrTopicNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Topic not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to get topic config: " + err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}

// UpdateTopicConfig 更新 Topic 配置
// @Summary 更新 Topic 配置
// @Description 更新指定 Topic 的配置参数
// @Tags kafka
// @Accept json
// @Produce json
// @Param topic path string true "Topic 名称"
// @Param request body map[string]string true "配置更新请求"
// @Success 200 {object} map[string]interface{} "配置更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "Topic 不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/topics/{topic}/config [put]
func (h *KafkaHandler) UpdateTopicConfig(c *gin.Context) {
	ctx := c.Request.Context()
	topicName := c.Param("topic")
	
	if topicName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "topic name is required",
		})
		return
	}
	
	var configs map[string]string
	if err := c.ShouldBindJSON(&configs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format: " + err.Error(),
		})
		return
	}
	
	if len(configs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "at least one config parameter is required",
		})
		return
	}
	
	err := h.kafkaService.UpdateTopicConfig(ctx, topicName, configs)
	if err != nil {
		if err == kafka.ErrTopicNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Topic not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to update topic config: " + err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Topic config updated successfully",
		"data": gin.H{
			"topic":   topicName,
			"configs": configs,
		},
	})
}

// GetTopicMetrics 获取 Topic 指标信息
// @Summary 获取 Topic 指标信息
// @Description 获取指定 Topic 的详细指标信息，包括消息数量、大小、吞吐量等
// @Tags kafka
// @Accept json
// @Produce json
// @Param topic path string true "Topic 名称"
// @Success 200 {object} map[string]interface{} "Topic 指标信息"
// @Failure 404 {object} map[string]interface{} "Topic 不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /kafka/topics/{topic}/metrics [get]
func (h *KafkaHandler) GetTopicMetrics(c *gin.Context) {
	ctx := c.Request.Context()
	topicName := c.Param("topic")
	
	if topicName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "topic name is required",
		})
		return
	}
	
	metrics, err := h.kafkaService.GetTopicMetrics(ctx, topicName)
	if err != nil {
		if err == kafka.ErrTopicNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Topic not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to get topic metrics: " + err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    metrics,
	})
}
