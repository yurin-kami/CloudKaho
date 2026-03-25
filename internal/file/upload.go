package file

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/yurin-kami/CloudKaho/service"
	"gorm.io/gorm"
)

// 请求结构体
type preCheckRequest struct {
	FileHash       string `json:"file_hash" binding:"required"`
	FileSize       int64  `json:"file_size" binding:"required"`
	FileName       string `json:"file_name" binding:"required"`
	ParentFolderID uint   `json:"parent_folder_id"`
}

type completePart struct {
	PartNumber int32  `json:"part_number" binding:"required"`
	ETag       string `json:"etag" binding:"required"`
}

type completeMultipartRequest struct {
	UploadID string         `json:"upload_id" binding:"required"`
	FileHash string         `json:"file_hash" binding:"required"`        //2次校验，防止客户端篡改 UploadID
	Parts    []completePart `json:"parts" binding:"required,dive,min=2"` // 最少2个分片，单分片请使用小文件接口
}

type partPresignRequest struct {
	UploadID   string `json:"upload_id" binding:"required"`
	PartNumber int32  `json:"part_number" binding:"required"` //分号从1开始
}

type smallFileUploadRequest struct {
	FileSize       int64  `json:"file_size" binding:"required"`
	FileName       string `json:"file_name" binding:"required"`
	ParentFolderID uint   `json:"parent_folder_id"`
}

func getUserIDFromContext(c *gin.Context) (uint, bool) {
	userIDValue, exists := c.Get("userID")
	if !exists {
		return 0, false
	}
	userID, ok := userIDValue.(uint)
	if !ok {
		return 0, false
	}
	return userID, true
}

func UploadSmallFile(fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		// 绑定表单字段（小文件使用 multipart/form-data）
		var req smallFileUploadRequest
		if err := c.ShouldBind(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request", "details": err.Error()})
			return
		}

		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "unauthorized"})
			return
		}

		// 从请求头获取文件内容（小文件建议使用 Base64 编码）
		// 注意：请求头有长度限制，仅适用于小文件
		fileContentBase64 := c.GetHeader("file")
		if fileContentBase64 == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "file header not found"})
			return
		}
		fileBytes, err := base64.StdEncoding.DecodeString(fileContentBase64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid file header", "details": err.Error()})
			return
		}

		// 校验文件大小，避免客户端伪造
		fileSize := int64(len(fileBytes))
		if req.FileSize > 0 && req.FileSize != fileSize {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "file size mismatch"})
			return
		}
		contentType := c.GetHeader("Content-Type")
		result, err := service.UploadSmallFile(ctx, fileConnection, userID, fileSize, req.FileName, req.ParentFolderID, fileBytes, contentType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error", "details": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    "0",
			"message": result.Message,
			"data": gin.H{
				"file_id": result.FileID,
				"url":     result.URL,
			},
		})
	}
}

func InitMultipartUpload(redisClient *redis.Client, fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		var req preCheckRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request", "details": err.Error()})
			return
		}

		// 获取用户 ID
		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "unauthorized"})
			return
		}

		result, err := service.InitMultipartUploadSession(ctx, fileConnection, redisClient, userID, req.FileHash, req.FileSize, req.FileName, req.ParentFolderID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error", "details": err.Error()})
			return
		}

		if result.Message == "quick_pass" {
			c.JSON(http.StatusOK, gin.H{
				"code":    "0",
				"message": "quick_pass",
				"data": gin.H{
					"file_id": result.FileID,
					"url":     result.URL,
				},
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    "0",
			"message": "init_success",
			"data": gin.H{
				"upload_id":  result.UploadID,
				"file_id":    result.FileID,
				"s3_key":     result.S3Key,
				"chunk_size": result.ChunkSize,
			},
		})
	}
}

func PartPresignURL(redisClient *redis.Client, fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		var req partPresignRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request", "details": err.Error()})
			return
		}

		// 获取用户 ID
		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "unauthorized"})
			return
		}

		result, err := service.GetPartPresignURL(ctx, redisClient, userID, req.UploadID, req.PartNumber)
		if err != nil {
			if errors.Is(err, service.ErrInvalid) {
				c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid upload ID"})
				return
			}
			if errors.Is(err, service.ErrForbidden) {
				c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "unauthorized"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to generate presigned URL", "details": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    "0",
			"message": "presign success",
			"data": gin.H{
				"presigned_url": result.PresignedURL,
				"part_number":   req.PartNumber,
				"upload_id":     req.UploadID,
				"expires_in":    result.ExpiresIn,
				"http_method":   result.HTTPMethod,
			},
		})
	}
}

func CompleteMultipartUpload(redisClient *redis.Client, fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		var req completeMultipartRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request", "details": err.Error()})
			return
		}

		// 获取用户 ID
		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "unauthorized"})
			return
		}

		parts := make([]service.CompletedPartInput, len(req.Parts))
		for i, part := range req.Parts {
			parts[i] = service.CompletedPartInput{
				PartNumber: part.PartNumber,
				ETag:       part.ETag,
			}
		}

		result, err := service.CompleteMultipartUploadSession(ctx, fileConnection, redisClient, userID, req.UploadID, req.FileHash, parts)
		if err != nil {
			if errors.Is(err, service.ErrInvalid) {
				c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": err.Error()})
				return
			}
			if errors.Is(err, service.ErrForbidden) {
				c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "unauthorized"})
				return
			}
			if errors.Is(err, service.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"code": "1", "error": err.Error()})
				return
			}
			log.Printf("transaction commit failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "transaction commit failed", "details": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    "0",
			"message": "upload complete",
			"data": gin.H{
				"file_id":   result.FileID,
				"url":       result.URL,
				"file_hash": result.FileHash,
			},
		})
	}
}
