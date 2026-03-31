package file

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/yurin-kami/CloudKaho/service"
	"gorm.io/gorm"
)

type DownloadRequest struct {
	FileID   uint   `json:"file_id" binding:"required,gt=0"`
	FileName string `json:"file_name" binding:"required"`
}

func DownloadFile(redisClient *redis.Client, fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		var req DownloadRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request", "details": err.Error()})
			return
		}
		req.FileName = strings.TrimSpace(req.FileName)
		if req.FileName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request", "details": "file_name cannot be empty"})
			return
		}

		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "unauthorized"})
			return
		}
		downloadURL, expiresIn, err := service.GetDownloadURLForUser(ctx, fileConnection, userID, req.FileID, req.FileName)
		if err != nil {
			if errors.Is(err, service.ErrForbidden) {
				c.JSON(http.StatusForbidden, gin.H{"code": "1", "error": "forbidden", "details": err.Error()})
				return
			}
			if errors.Is(err, service.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"code": "1", "error": "not found", "details": err.Error()})
				return
			}
			if errors.Is(err, service.ErrInvalid) {
				c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request", "details": err.Error()})
				return
			}
			log.Printf("download file internal error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "internal error", "details": "internal server error"})
			return
		}
		//重定向到预签名URL
		// c.Redirect(http.StatusFound, presignResult.URL)
		//或者直接返回预签名URL给前端，由前端发起下载请求
		c.JSON(http.StatusOK, gin.H{
			"code":    "0",
			"message": "success",
			"data": gin.H{
				"download_url": downloadURL,
				"file_name":    req.FileName,
				"expires_in":   expiresIn.Seconds(),
			},
		})
	}
}
