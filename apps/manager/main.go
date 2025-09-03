package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sysarmor/sysarmor/apps/manager/api/handlers"
	"github.com/sysarmor/sysarmor/apps/manager/config"
	"github.com/sysarmor/sysarmor/apps/manager/storage"
	
	// Swagger imports
	"github.com/swaggo/gin-swagger"
	"github.com/swaggo/files"
	_ "github.com/sysarmor/sysarmor/apps/manager/docs" // Swagger docs
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
		collectors.POST("/:id/heartbeat", collectorHandler.Heartbeat)      // 心跳上报
		collectors.POST("/:id/probe", collectorHandler.ProbeHeartbeat)     // 主动探测
		
		// 元数据管理路由
		collectors.PUT("/:id/metadata", collectorHandler.UpdateMetadata)
		
		// 删除和注销路由
		collectors.DELETE("/:id", collectorHandler.Delete)           // 删除 Collector (支持 force 参数)
		collectors.POST("/:id/unregister", collectorHandler.Unregister) // 注销 Collector (软删除)
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
	healthHandler := handlers.NewHealthHandler(db.DB())
	health := api.Group("/health")
	{
		health.GET("", healthHandler.GetHealthOverview)                    // 新的综合健康状态概览
		health.GET("/comprehensive", healthHandler.GetComprehensiveHealth) // 详细的系统健康状态
		health.GET("/system", healthHandler.GetSystemHealth)
		health.GET("/workers", healthHandler.GetWorkers)
		health.GET("/workers/healthy", healthHandler.GetHealthyWorkers)
		health.GET("/workers/select", healthHandler.SelectWorker)
		health.GET("/workers/:name", healthHandler.GetWorkerDetails)
		health.GET("/workers/:name/metrics", healthHandler.GetWorkerMetrics)
		health.GET("/workers/:name/components", healthHandler.GetWorkerComponents)
	}

	// 事件查询路由
	kafkaBrokers := cfg.GetKafkaBrokerList() // 从配置文件读取
	eventsHandler := handlers.NewEventsHandler(kafkaBrokers)
	events := api.Group("/events")
	{
		// 通用事件查询
		events.GET("/query", eventsHandler.QueryEvents)
		events.GET("/latest", eventsHandler.GetLatestEvents)
		events.POST("/search", eventsHandler.SearchEvents)
		
		// Collector 相关事件查询
		events.GET("/collectors/:collector_id", eventsHandler.QueryCollectorEvents)
		events.GET("/collectors/topics", eventsHandler.GetCollectorTopics)
		
		// Topic 管理
		events.GET("/topics", eventsHandler.ListTopics)
		events.GET("/topics/:topic/info", eventsHandler.GetTopicInfo)
	}

	// 服务管理路由组
	services := api.Group("/services")

	// Kafka 管理路由
	kafkaHandler := handlers.NewKafkaHandler(kafkaBrokers)
	kafka := services.Group("/kafka")
	{
		// 连接测试
		kafka.GET("/test-connection", kafkaHandler.TestKafkaConnection)
		
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
		// 连接测试
		flink.GET("/test-connection", flinkHandler.TestFlinkConnection)
		
		// 集群管理
		flink.GET("/overview", flinkHandler.GetClusterOverview)
		flink.GET("/config", flinkHandler.GetConfig)
		flink.GET("/health", flinkHandler.GetClusterHealth)
		
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
		cfg.GetOpenSearchURL(),     // 从配置文件读取 OpenSearch URL
		cfg.GetOpenSearchUsername(), // 从配置文件读取用户名
		cfg.GetOpenSearchPassword(), // 从配置文件读取密码
	)
	log.Printf("✅ OpenSearch handler initialized successfully")
	
	if opensearchHandler != nil {
		opensearch := services.Group("/opensearch")
		{
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
