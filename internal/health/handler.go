package health

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Handler 健康检查处理器
type Handler struct {
	db       *sql.DB
	redis    *redis.Client
	s3Client *s3.Client
	ready    atomic.Bool
}

// NewHandler 创建健康检查处理器
func NewHandler(db *sql.DB, redis *redis.Client, s3Client *s3.Client) *Handler {
	h := &Handler{
		db:       db,
		redis:    redis,
		s3Client: s3Client,
	}
	h.ready.Store(false) // 初始为未就绪
	return h
}

// SetReady 标记应用就绪状态
func (h *Handler) SetReady(ready bool) {
	h.ready.Store(ready)
}

// Liveness 存活检查 (仅检查进程响应)
func (h *Handler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
	})
}

// Readiness 就绪检查 (检查所有依赖)
func (h *Handler) Readiness(c *gin.Context) {
	if !h.ready.Load() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "not_ready",
			"message": "application initializing",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// 并发执行所有检查
	var wg sync.WaitGroup
	checks := make(map[string]CheckResult)
	mu := sync.Mutex{}

	// MySQL 检查
	wg.Add(1)
	go func() {
		defer wg.Done()
		result := h.checkMySQL(ctx)
		mu.Lock()
		checks["mysql"] = result
		mu.Unlock()
	}()

	// Redis 检查
	wg.Add(1)
	go func() {
		defer wg.Done()
		result := h.checkRedis(ctx)
		mu.Lock()
		checks["redis"] = result
		mu.Unlock()
	}()

	// S3 检查 (可选,故障时降级为 warn)
	wg.Add(1)
	go func() {
		defer wg.Done()
		result := h.checkS3(ctx)
		mu.Lock()
		checks["s3"] = result
		mu.Unlock()
	}()

	wg.Wait()

	// 汇总状态
	overallStatus := "ok"
	for name, check := range checks {
		if check.Status == "fail" && name != "s3" {
			// S3 故障不影响整体状态 (降级为 warn)
			overallStatus = "unavailable"
			break
		} else if check.Status == "warn" && overallStatus != "unavailable" {
			overallStatus = "degraded"
		}
	}

	statusCode := http.StatusOK
	if overallStatus == "unavailable" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, HealthResponse{
		Status:    overallStatus,
		Timestamp: time.Now().Unix(),
		Checks:    checks,
	})
}

// Startup 启动检查
func (h *Handler) Startup(c *gin.Context) {
	if !h.ready.Load() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "starting",
			"message": "application initializing",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "started",
	})
}

// checkMySQL 检查 MySQL 连接
func (h *Handler) checkMySQL(ctx context.Context) CheckResult {
	start := time.Now()

	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := h.db.PingContext(checkCtx); err != nil {
		return CheckResult{
			Status:   "fail",
			Duration: time.Since(start).Milliseconds(),
			Message:  "ping failed",
		}
	}

	return CheckResult{
		Status:   "pass",
		Duration: time.Since(start).Milliseconds(),
	}
}

// checkRedis 检查 Redis 连接
func (h *Handler) checkRedis(ctx context.Context) CheckResult {
	start := time.Now()

	checkCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	if err := h.redis.Ping(checkCtx).Err(); err != nil {
		return CheckResult{
			Status:   "fail",
			Duration: time.Since(start).Milliseconds(),
			Message:  "ping failed",
		}
	}

	return CheckResult{
		Status:   "pass",
		Duration: time.Since(start).Milliseconds(),
	}
}

// checkS3 检查 S3 连接 (轻量级,仅列举 buckets)
func (h *Handler) checkS3(ctx context.Context) CheckResult {
	start := time.Now()

	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := h.s3Client.ListBuckets(checkCtx, &s3.ListBucketsInput{})
	if err != nil {
		return CheckResult{
			Status:   "warn", // S3 故障降级为 warn
			Duration: time.Since(start).Milliseconds(),
			Message:  "list buckets failed",
		}
	}

	return CheckResult{
		Status:   "pass",
		Duration: time.Since(start).Milliseconds(),
	}
}
