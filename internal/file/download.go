package file

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/yurin-kami/CloudKaho/models"
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
		//检查用户是否有权限下载这个文件
		var relation models.UserFileRelation
		if err := fileConnection.WithContext(ctx).Preload("FileMeta").
			Where("user_id = ? AND file_meta_id = ? AND is_deleted = ?", userID, req.FileID, false).
			First(&relation).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusForbidden, gin.H{"code": "1", "error": "file not found or no permission"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error"})
			}
			return
		}

		//检查文件元状态信息
		var fileMeta models.FileMeta
		if err := fileConnection.WithContext(ctx).Where("file_id = ?", req.FileID).First(&fileMeta).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusForbidden, gin.H{"code": "1", "error": "file not found"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error"})
			}
			return
		}
		if fileMeta.Status != 1 {
			c.JSON(http.StatusForbidden, gin.H{"code": "1", "error": "file is not available for download (status not active)"})
			return
		}

		//生成预签名URL
		s3Client, err := newS3Client(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to create s3 client"})
			return
		}

		presignClient := s3.NewPresignClient(s3Client)
		presignExpires := 15 * time.Minute
		presignResult, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket:                     aws.String(""), //TODO: load bucket name from config
			Key:                        aws.String(fileMeta.StoragePath),
			ResponseContentDisposition: aws.String(fmt.Sprintf(`attachment; filename="%s"`, url.QueryEscape(req.FileName))),
		}, s3.WithPresignExpires(presignExpires))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to generate download url", "details": err.Error()})
			return
		}
		//重定向到预签名URL
		// c.Redirect(http.StatusFound, presignResult.URL)
		//或者直接返回预签名URL给前端，由前端发起下载请求
		c.JSON(http.StatusOK, gin.H{
			"code":    "0",
			"message": "success",
			"data": gin.H{
				"download_url": presignResult.URL,
				"file_name":    req.FileName,
				"expires_in":   presignExpires.Seconds(),
			},
		})
	}
}
