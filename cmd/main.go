package main

import (
	"context"
	_ "database/sql" // 用于健康检查的 *sql.DB 类型
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	cfg "github.com/yurin-kami/CloudKaho/config"
	"github.com/yurin-kami/CloudKaho/internal/health"
	"github.com/yurin-kami/CloudKaho/internal/routes"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	appCfg := cfg.MustLoad()

	// 1. 初始化 GORM 连接
	fileConnection, err := gorm.Open(mysql.Open(appCfg.DB.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// 2. 获取底层 *sql.DB (用于健康检查)
	sqlDB, err := fileConnection.DB()
	if err != nil {
		log.Fatalf("failed to get database instance: %v", err)
	}

	// 3. 初始化 Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     appCfg.Redis.Addr,
		Password: appCfg.Redis.Password,
		DB:       appCfg.Redis.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to connect redis: %v", err)
	}

	// 4. 初始化 S3 客户端
	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = &appCfg.S3.Endpoint
		o.UsePathStyle = appCfg.S3.UsePathStyle
	})

	// 5. 创建健康检查 handler
	healthHandler := health.NewHandler(sqlDB, redisClient, s3Client)

	// 6. 初始化路由
	router := gin.Default()

	// 注册健康检查路由 (优先注册,确保可用性)
	routes.HealthRoute(router, healthHandler)

	// 注册业务路由
	routes.UserRoute(router)
	routes.FileRoute(router, redisClient, fileConnection)

	// 7. 启动清理任务
	StartCleanupJob(fileConnection, redisClient)

	// 8. 标记应用为就绪
	healthHandler.SetReady(true)
	log.Println("Application initialized successfully")

	// 9. 启动服务器
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
