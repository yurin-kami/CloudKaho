package file

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/yurin-kami/CloudKaho/service"
	"gorm.io/gorm"
)

type DownloadRequest struct {
	FileID   uint   `json:"file_id" validate:"required"`
	FileName string `json:"file_name" validate:"required"`
}

func DownloadFile(redisClient *redis.Client, fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		var req DownloadRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
			return
		}

		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		downloadURL, expiresIn, err := service.GetDownloadURLForUser(ctx, fileConnection, userID, req.FileID, req.FileName)
		if err != nil {
			if errors.Is(err, service.ErrForbidden) {
				c.JSON(http.StatusForbidden, gin.H{"code": "1", "error": "file not found or no permission"})
				return
			}
			if errors.Is(err, service.ErrNotFound) {
				c.JSON(http.StatusForbidden, gin.H{"code": "1", "error": "file not found"})
				return
			}
			if errors.Is(err, service.ErrInvalid) {
				c.JSON(http.StatusForbidden, gin.H{"code": "1", "error": "file is not available for download (status not active)"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error"})
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
