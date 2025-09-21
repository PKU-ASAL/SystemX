package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sysarmor/sysarmor/apps/manager/api/handlers"
	"github.com/sysarmor/sysarmor/apps/manager/config"
	"github.com/sysarmor/sysarmor/apps/manager/services/wazuh"
	"github.com/sysarmor/sysarmor/apps/manager/storage"

	// Swagger imports
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	docs "github.com/sysarmor/sysarmor/apps/manager/docs" // Swagger docs
)

// @title SysArmor Manager API
// @version 1.0
// @description SysArmor EDR 系统的控制平面服务 API
// @termsOfService https://sysarmor.com/terms

// @contact.name SysArmor Team
// @contact.url https://sysarmor.com/support
// @contact.email support@sysarmor.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /api/v1

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// 动态设置Swagger文档的Host
	if cfg.ExternalURL != "" {
		// 解析外部URL以提取host部分
		if host := extractHostFromURL(cfg.ExternalURL); host != "" {
			docs.SwaggerInfo.Host = host
		}
	} else {
		docs.SwaggerInfo.Host = fmt.Sprintf("localhost:%d", cfg.Port)
	}

	// 连接数据库
	db, err := storage.NewDatabase()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// 创建 Gin 路由
	r := gin.Default()

	// 健康检查端点
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":   "healthy",
			"service":  "sysarmor-manager",
			"version":  "1.0.0",
			"database": "connected",
		})
	})

	// API 路由组
	api := r.Group("/api/v1")

	// Collector 相关路由
	collectorHandler := handlers.NewCollectorHandler(db.DB())
	collectors := api.Group("/collectors")
	{
		collectors.POST("/register", collectorHandler.Register)
		collectors.GET("/:id", collectorHandler.GetStatus)
		collectors.GET("", collectorHandler.ListCollectors) // 支持 Query Parameters 过滤

		// Nova分支新增: 双向心跳路由
		collectors.POST("/:id/heartbeat", collectorHandler.Heartbeat)  // 心跳上报
		collectors.POST("/:id/probe", collectorHandler.ProbeHeartbeat) // 主动探测

		// 元数据管理路由
		collectors.PUT("/:id/metadata", collectorHandler.UpdateMetadata)

		// 注销和删除路由 (注意顺序：具体路径在前，通用路径在后)
		collectors.POST("/:id/unregister", collectorHandler.Unregister) // 注销 Collector (软删除)
		collectors.DELETE("/:id", collectorHandler.Delete)              // 删除 Collector (支持 force 参数)
	}

	// 资源管理路由 (统一的脚本、配置、二进制文件 API)
	resourcesHandler := handlers.NewResourcesHandler(db.DB())
	resources := api.Group("/resources")
	{
		// 脚本资源 (动态生成)
		resources.GET("/scripts/:deployment_type/:script_name", resourcesHandler.GetScript)

		// 二进制资源 (静态下载)
		resources.GET("/binaries/:filename", resourcesHandler.GetBinary)

		// 配置资源 (动态生成)
		resources.GET("/configs/:deployment_type/:config_name", resourcesHandler.GetConfig)
	}

	// 健康检查路由
	healthHandler := handlers.NewHealthHandler(db.DB(), cfg)
	health := api.Group("/health")
	{
		health.GET("", healthHandler.GetHealthOverview)                    // 映射到 /api/v1/health
		health.GET("/overview", healthHandler.GetHealthOverview)           // 映射到 /api/v1/health/overview
		health.GET("/comprehensive", healthHandler.GetComprehensiveHealth) // 详细的系统健康状态
		health.GET("/system", healthHandler.GetSystemHealth)
		health.GET("/workers", healthHandler.GetWorkers)
		health.GET("/workers/healthy", healthHandler.GetHealthyWorkers)
		health.GET("/workers/select", healthHandler.SelectWorker)
		health.GET("/workers/:name", healthHandler.GetWorkerDetails)
		health.GET("/workers/:name/metrics", healthHandler.GetWorkerMetrics)
		health.GET("/workers/:name/components", healthHandler.GetWorkerComponents)
	}

	// 事件查询路由（MVP简化版本）
	kafkaBrokers := cfg.GetKafkaBrokerList() // 从配置文件读取
	eventsHandler := handlers.NewEventsHandler(kafkaBrokers)
	events := api.Group("/events")
	{
		// 通用事件查询接口
		events.GET("/latest", eventsHandler.GetLatestEvents)
		events.GET("/query", eventsHandler.QueryEvents)
		events.POST("/search", eventsHandler.SearchEvents)
		
		// Topic 管理
		events.GET("/topics", eventsHandler.ListTopics)
		events.GET("/topics/:topic/info", eventsHandler.GetTopicInfo)
		
		// 保留collector特定接口（向后兼容，但标记为deprecated）
		events.GET("/collectors/:collector_id", eventsHandler.QueryCollectorEvents)
		events.GET("/collectors/topics", eventsHandler.GetCollectorTopics)
	}

	// Topic配置管理路由（新增）
	topicsHandler := handlers.NewTopicsHandler()
	topics := api.Group("/topics")
	{
		// Topic配置查询
		topics.GET("/configs", topicsHandler.GetTopicConfigs)
		topics.GET("/categories", topicsHandler.GetTopicsByCategory)
		topics.GET("/defaults", topicsHandler.GetDefaultTopics)
		
		// Topic验证和分区信息
		topics.GET("/:topic/validate", topicsHandler.ValidateTopic)
		topics.GET("/:topic/partitions", topicsHandler.GetTopicPartitions)
	}

	// 服务管理路由组
	services := api.Group("/services")

	// Wazuh 集成路由 (HFW分支新增)
	log.Printf("🛡️ Initializing Wazuh service...")
	wazuhService, err := wazuh.NewWazuhService(cfg)
	if err != nil {
		log.Printf("❌ Failed to initialize Wazuh service: %v", err)
	} else {
		wazuhHandler := handlers.NewWazuhHandler(wazuhService)

		// 完整的Wazuh路由注册
		wazuhGroup := api.Group("/wazuh")
		{
			config := wazuhGroup.Group("/config")
			{
				config.GET("", wazuhHandler.GetConfig)
				config.PUT("", wazuhHandler.UpdateConfig)
				config.POST("/validate", wazuhHandler.ValidateConfig)
				config.POST("/reload", wazuhHandler.ReloadConfig)
			}

			manager := wazuhGroup.Group("/manager")
			{
				manager.GET("/info", wazuhHandler.GetManagerInfo)
				manager.GET("/status", wazuhHandler.GetManagerStatus)
				manager.GET("/logs", wazuhHandler.GetManagerLogs)
				manager.GET("/stats", wazuhHandler.GetManagerStats)
				manager.POST("/restart", wazuhHandler.RestartManager)
				manager.GET("/configuration", wazuhHandler.GetManagerConfiguration)
			}

			// Agent管理
			agents := wazuhGroup.Group("/agents")
			{
				agents.GET("", wazuhHandler.GetAgents)
				agents.POST("", wazuhHandler.AddAgent)
				agents.GET("/:id", wazuhHandler.GetAgent)
				agents.PUT("/:id", wazuhHandler.UpdateAgent)
				agents.DELETE("/:id", wazuhHandler.DeleteAgent)
				agents.POST("/:id/restart", wazuhHandler.RestartAgent)
				agents.GET("/:id/key", wazuhHandler.GetAgentKey)
				agents.POST("/:id/upgrade", wazuhHandler.UpgradeAgent)

				// Agent详细信息
				agents.GET("/:id/system", wazuhHandler.GetAgentSystem)
				agents.GET("/:id/hardware", wazuhHandler.GetAgentHardware)
				agents.GET("/:id/ports", wazuhHandler.GetAgentPorts)
				agents.GET("/:id/packages", wazuhHandler.GetAgentPackages)
				agents.GET("/:id/processes", wazuhHandler.GetAgentProcesses)
				agents.GET("/:id/netproto", wazuhHandler.GetAgentNetworkProtocols)
				agents.GET("/:id/netaddr", wazuhHandler.GetAgentNetworkAddresses)

				// Agent统计信息
				agents.GET("/:id/stats/logcollector", wazuhHandler.GetAgentLogcollectorStats)
				agents.GET("/:id/daemons/stats", wazuhHandler.GetAgentDaemonStats)

				// 安全扫描
				agents.GET("/:id/ciscat", wazuhHandler.GetAgentCiscatResults)
				agents.GET("/:id/sca", wazuhHandler.GetAgentSCAResults)
				agents.GET("/:id/rootcheck", wazuhHandler.GetAgentRootcheckResults)
				agents.DELETE("/:id/rootcheck", wazuhHandler.ClearAgentRootcheckResults)
				agents.GET("/:id/rootcheck/last_scan", wazuhHandler.GetAgentRootcheckLastScan)

				// Agent高级操作
				agents.PUT("/:id/active-response", wazuhHandler.ExecuteActiveResponse)
				agents.GET("/:id/upgrade/result", wazuhHandler.GetUpgradeResult)
			}

			// 批量Agent操作
			wazuhGroup.PUT("/agents/upgrade", wazuhHandler.UpgradeAgents)
			wazuhGroup.PUT("/agents/upgrade/custom", wazuhHandler.CustomUpgradeAgents)
			wazuhGroup.PUT("/rootcheck", wazuhHandler.RunRootcheck)

			// 集群和概览
			wazuhGroup.GET("/cluster/health", wazuhHandler.GetClusterHealth)
			wazuhGroup.GET("/overview/agents", wazuhHandler.GetOverviewAgents)

			// 组管理
			groups := wazuhGroup.Group("/groups")
			{
				groups.GET("", wazuhHandler.GetGroups)
				groups.POST("", wazuhHandler.CreateGroup)
				groups.GET("/:name", wazuhHandler.GetGroup)
				groups.PUT("/:name", wazuhHandler.UpdateGroup)
				groups.DELETE("/:name", wazuhHandler.DeleteGroup)
				groups.GET("/:name/agents", wazuhHandler.GetGroupAgents)
				groups.POST("/:name/agents", wazuhHandler.AddAgentToGroup)
				groups.DELETE("/:name/agents/:agent_id", wazuhHandler.RemoveAgentFromGroup)
				groups.GET("/:name/configuration", wazuhHandler.GetGroupConfiguration)
				groups.PUT("/:name/configuration", wazuhHandler.UpdateGroupConfiguration)
			}

			// 规则管理
			rules := wazuhGroup.Group("/rules")
			{
				rules.GET("", wazuhHandler.GetRules)
				rules.GET("/:id", wazuhHandler.GetRule)
				rules.POST("", wazuhHandler.CreateRule)
				rules.PUT("/:id", wazuhHandler.UpdateRule)
				rules.DELETE("/:id", wazuhHandler.DeleteRule)
				rules.GET("/files", wazuhHandler.GetRuleFiles)
				rules.GET("/files/:filename", wazuhHandler.GetRuleFile)
				rules.PUT("/files/:filename", wazuhHandler.UpdateRuleFile)
			}

			// Indexer API
			indexer := wazuhGroup.Group("/indexer")
			{
				indexer.GET("/health", wazuhHandler.GetIndexerHealth)
				indexer.GET("/info", wazuhHandler.GetIndexerInfo)
				indexer.GET("/indices", wazuhHandler.GetIndices)
				indexer.POST("/indices", wazuhHandler.CreateIndex)
				indexer.DELETE("/indices/:name", wazuhHandler.DeleteIndex)
				indexer.GET("/templates", wazuhHandler.GetIndexTemplates)
				indexer.POST("/templates", wazuhHandler.CreateIndexTemplate)
			}

			// 告警查询
			alerts := wazuhGroup.Group("/alerts")
			{
				alerts.POST("/search", wazuhHandler.SearchAlerts)
				alerts.GET("/agent/:id", wazuhHandler.GetAlertsByAgent)
				alerts.GET("/rule/:id", wazuhHandler.GetAlertsByRule)
				alerts.GET("/level/:level", wazuhHandler.GetAlertsByLevel)
				alerts.POST("/aggregate", wazuhHandler.AggregateAlerts)
				alerts.GET("/stats", wazuhHandler.GetAlertStats)
			}

			// 监控和统计
			monitoring := wazuhGroup.Group("/monitoring")
			{
				monitoring.GET("/overview", wazuhHandler.GetMonitoringOverview)
				monitoring.GET("/agents/summary", wazuhHandler.GetAgentsSummary)
				monitoring.GET("/alerts/summary", wazuhHandler.GetAlertsSummary)
				monitoring.GET("/system/stats", wazuhHandler.GetSystemStats)
			}
		}

		log.Printf("✅ Wazuh routes registered successfully")
	}

	// Kafka 管理路由
	kafkaHandler := handlers.NewKafkaHandler(kafkaBrokers)
	kafka := services.Group("/kafka")
	{
		// 健康检查
		kafka.GET("/health", kafkaHandler.GetKafkaHealth)

		// 集群管理
		kafka.GET("/clusters", kafkaHandler.GetClusters)

		// Broker 管理
		kafka.GET("/brokers", kafkaHandler.GetBrokers)
		kafka.GET("/brokers/overview", kafkaHandler.GetBrokersOverview) // 新增：Brokers 概览

		// Topic 管理
		kafka.GET("/topics", kafkaHandler.GetTopics)
		kafka.GET("/topics/overview", kafkaHandler.GetTopicsOverview) // 新增：Topics 概览
		kafka.POST("/topics", kafkaHandler.CreateTopic)
		kafka.GET("/topics/:topic", kafkaHandler.GetTopicDetails)
		kafka.DELETE("/topics/:topic", kafkaHandler.DeleteTopic)
		kafka.GET("/topics/:topic/messages", kafkaHandler.GetTopicMessages)

		// Topic 配置管理
		kafka.GET("/topics/:topic/config", kafkaHandler.GetTopicConfig)
		kafka.PUT("/topics/:topic/config", kafkaHandler.UpdateTopicConfig)

		// Topic 指标管理
		kafka.GET("/topics/:topic/metrics", kafkaHandler.GetTopicMetrics)

		// Consumer Group 管理
		kafka.GET("/consumer-groups", kafkaHandler.GetConsumerGroups)
		kafka.GET("/consumer-groups/:group", kafkaHandler.GetConsumerGroupDetails)
	}

	// Flink 管理路由
	log.Printf("🔧 Initializing Flink handler with URL: %s", cfg.GetFlinkURL())

	flinkHandler := handlers.NewFlinkHandler(cfg.GetFlinkURL())
	flink := services.Group("/flink")
	{
		// 健康检查
		flink.GET("/health", flinkHandler.GetFlinkHealth)

		// 集群管理
		flink.GET("/overview", flinkHandler.GetClusterOverview)
		flink.GET("/config", flinkHandler.GetConfig)
		flink.GET("/cluster/health", flinkHandler.GetClusterHealth)

		// 作业管理
		flink.GET("/jobs", flinkHandler.GetJobs)
		flink.GET("/jobs/overview", flinkHandler.GetJobsOverview)
		flink.GET("/jobs/:job_id", flinkHandler.GetJobDetails)
		flink.GET("/jobs/:job_id/metrics", flinkHandler.GetJobMetrics)

		// TaskManager 管理
		flink.GET("/taskmanagers", flinkHandler.GetTaskManagers)
		flink.GET("/taskmanagers/overview", flinkHandler.GetTaskManagersOverview)
	}
	log.Printf("✅ Flink routes registered successfully")

	// OpenSearch 管理路由
	log.Printf("🔍 Initializing OpenSearch handler with URL: %s", cfg.GetOpenSearchURL())
	log.Printf("🔍 OpenSearch Username: %s", cfg.GetOpenSearchUsername())
	log.Printf("🔍 About to call handlers.NewOpenSearchHandler...")

	opensearchHandler := handlers.NewOpenSearchHandler(
		cfg.GetOpenSearchURL(),      // 从配置文件读取 OpenSearch URL
		cfg.GetOpenSearchUsername(), // 从配置文件读取用户名
		cfg.GetOpenSearchPassword(), // 从配置文件读取密码
	)
	log.Printf("✅ OpenSearch handler initialized successfully")

	if opensearchHandler != nil {
		opensearch := services.Group("/opensearch")
		{
			// 健康检查
			opensearch.GET("/health", opensearchHandler.GetOpenSearchHealth)

			// 集群管理
			cluster := opensearch.Group("/cluster")
			{
				cluster.GET("/health", opensearchHandler.GetClusterHealth)
				cluster.GET("/stats", opensearchHandler.GetClusterStats)
			}

			// 索引管理
			opensearch.GET("/indices", opensearchHandler.GetIndices)

			// 事件搜索和查询
			events := opensearch.Group("/events")
			{
				events.GET("/search", opensearchHandler.SearchEvents)
				events.GET("/time-range", opensearchHandler.GetEventsByTimeRange)
				events.GET("/high-risk", opensearchHandler.GetEventsByRiskScore)
				events.GET("/by-source", opensearchHandler.GetEventsBySource)
				events.GET("/threats", opensearchHandler.GetThreatEvents)
				events.GET("/recent", opensearchHandler.GetRecentEvents)
				events.GET("/aggregations", opensearchHandler.GetEventAggregations)
			}
		}
		log.Printf("✅ OpenSearch routes registered successfully")
	} else {
		log.Printf("❌ OpenSearch handler is nil, skipping route registration")
	}

	// Swagger 文档路由
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// API 文档重定向
	r.GET("/docs", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})

	port := cfg.Port
	if port == 0 {
		port = 8080
	}

	log.Printf("🚀 SysArmor Manager starting on port %d", port)
	log.Printf("📋 Health check: http://localhost:%d/health", port)
	log.Printf("📖 API docs: http://localhost:%d/swagger/index.html", port)
	log.Printf("🔗 Docs redirect: http://localhost:%d/docs", port)

	if err := r.Run(fmt.Sprintf(":%d", port)); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

// extractHostFromURL 从URL中提取host部分 (hostname:port)
func extractHostFromURL(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}

	// 如果URL中没有端口，根据协议添加默认端口
	host := u.Host
	if !strings.Contains(host, ":") {
		switch u.Scheme {
		case "http":
			host += ":80"
		case "https":
			host += ":443"
		}
	}

	return host
}
