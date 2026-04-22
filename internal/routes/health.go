package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/internal/health"
)

// HealthRoute 注册健康检查路由
func HealthRoute(router *gin.Engine, handler *health.Handler) {
	router.GET("/healthz", handler.Liveness)
	router.GET("/readyz", handler.Readiness)
	router.GET("/startupz", handler.Startup)

	// 保留 /api/health 作为 readiness 的别名 (向后兼容)
	router.GET("/api/health", handler.Readiness)
}
