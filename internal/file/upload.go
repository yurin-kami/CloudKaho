package file

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/yurin-kami/CloudKaho/models"
	"gorm.io/gorm"
)

const (
	UploadStatusActive    = 1
	UploadStatusUploading = 2
	UploadStatusDeleted   = 3
	ChunkSize             = 5 * 1024 * 1024 // 5MB
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

// 查询已存在且处于可用状态的文件元信息
func findActiveFileByHash(ctx context.Context, tx *gorm.DB, fileHash string) (*models.FileMeta, error) {
	var existingFile models.FileMeta
	// 使用上下文，避免长事务阻塞
	err := tx.WithContext(ctx).
		Where("file_hash = ? AND status = ?", fileHash, UploadStatusActive).
		First(&existingFile).Error
	if err != nil {
		return nil, err
	}
	return &existingFile, nil
}

// 增加文件引用计数
func incrementReferenceCount(ctx context.Context, tx *gorm.DB, fileMeta *models.FileMeta) error {
	// 使用上下文，确保请求取消时能及时终止
	return tx.WithContext(ctx).
		Model(fileMeta).
		Update("reference_count", gorm.Expr("reference_count + ?", 1)).Error
}

// 创建用户与文件的关联记录
func createUserFileRelation(ctx context.Context, tx *gorm.DB, userID uint, fileMetaID uint, fileName string, parentFolderID uint) error {
	userFile := models.UserFileRelation{
		UserID:         userID,
		FileMetaID:     fileMetaID,
		FileName:       fileName,
		ParentFolderID: parentFolderID,
		IsDeleted:      false,
	}
	// 使用上下文，确保与外层事务一致
	return tx.WithContext(ctx).Create(&userFile).Error
}

// 创建文件元信息记录
func createFileMeta(ctx context.Context, tx *gorm.DB, req preCheckRequest, s3Key string) (*models.FileMeta, error) {
	newFileMeta := models.FileMeta{
		FileHash:       req.FileHash,
		FileSize:       req.FileSize,
		ReferenceCount: 1,
		StoragePath:    s3Key,
		Status:         UploadStatusUploading, // 2 = Uploading
		// 如果有 UploadID 字段，也可以存进去方便调试
	}
	// 使用上下文，避免长时间占用连接
	if err := tx.WithContext(ctx).Create(&newFileMeta).Error; err != nil {
		return nil, err
	}
	return &newFileMeta, nil
}

// 创建小文件的元信息记录（状态为 Active）
func createActiveFileMeta(ctx context.Context, tx *gorm.DB, fileHash string, fileSize int64, s3Key string) (*models.FileMeta, error) {
	newFileMeta := models.FileMeta{
		FileHash:       fileHash,
		FileSize:       fileSize,
		ReferenceCount: 1,
		StoragePath:    s3Key,
		Status:         UploadStatusActive,
	}
	// 使用上下文，确保请求取消时能及时终止
	if err := tx.WithContext(ctx).Create(&newFileMeta).Error; err != nil {
		return nil, err
	}
	return &newFileMeta, nil
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

// TODO: 完善 S3 客户端配置
func newS3Client(ctx context.Context) (*s3.Client, error) {
	// 加载默认配置 (会从环境变量 AWS_ACCESS_KEY_ID 等读取)
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("cn-east-1"), // 替换为中国科技云的实际区域
	)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		// 中国科技云或其他兼容 S3 服务通常需要自定义 Endpoint
		o.BaseEndpoint = aws.String("https://s3drive.cstcloud.cn")
		o.UsePathStyle = true // 开启 Path Style (http://bucket.endpoint)
	}), nil
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
		fileHashBytes := sha256.Sum256(fileBytes)
		fileHash := hex.EncodeToString(fileHashBytes[:])

		tx := fileConnection.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()

		// 如果文件存在则添加引用关系，否则直接上传
		existingFile, err := findActiveFileByHash(ctx, tx, fileHash)
		if err == nil {
			if err := incrementReferenceCount(ctx, tx, existingFile); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to update reference count"})
				return
			}
			if err := createUserFileRelation(ctx, tx, userID, existingFile.FileID, req.FileName, req.ParentFolderID); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to create user file relation"})
				return
			}

			tx.Commit()
			c.JSON(http.StatusOK, gin.H{
				"code":    "0",
				"message": "quick_pass",
				"data": gin.H{
					"file_id": existingFile.FileID,
					"url":     existingFile.StoragePath,
				},
			})
			return
		}

		if err != gorm.ErrRecordNotFound {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error", "details": err.Error()})
			return
		}

		// 生成 S3 Key (例如: files/ab/abcd1234...)
		s3Key := fmt.Sprintf("files/%s/%s", fileHash[:2], fileHash)

		// 初始化 S3 客户端并上传
		s3Client, err := newS3Client(ctx)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "s3 client init failed"})
			return
		}

		contentType := c.GetHeader("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String("your-bucket-name"), // TODO: load from config
			Key:           aws.String(s3Key),
			Body:          bytes.NewReader(fileBytes),
			ContentLength: aws.Int64(fileSize),
			ContentType:   aws.String(contentType),
		})
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "s3 upload failed", "details": err.Error()})
			return
		}

		// 写入文件元信息与用户关联
		newFileMeta, err := createActiveFileMeta(ctx, tx, fileHash, fileSize, s3Key)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to create file meta"})
			return
		}
		if err := createUserFileRelation(ctx, tx, userID, newFileMeta.FileID, req.FileName, req.ParentFolderID); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to create user file relation"})
			return
		}

		tx.Commit()
		c.JSON(http.StatusOK, gin.H{
			"code":    "0",
			"message": "upload_success",
			"data": gin.H{
				"file_id": newFileMeta.FileID,
				"url":     newFileMeta.StoragePath,
			},
		})
	}
}

func InitMultipartUpload(redisClient *redis.Client, fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		tx := fileConnection.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()

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

		// 1. 查询文件是否存在 (必须检查状态为 Active，1=Active, 2=Uploading)
		existingFile, err := findActiveFileByHash(ctx, tx, req.FileHash)
		if err == nil {
			// ================= 情况 A: 文件已存在且健康 (秒传) =================

			// 1.1 增加引用计数
			if err := incrementReferenceCount(ctx, tx, existingFile); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to update reference count"})
				return
			}

			// 1.2 创建用户关联记录
			if err := createUserFileRelation(ctx, tx, userID, existingFile.FileID, req.FileName, req.ParentFolderID); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to create user file relation"})
				return
			}

			tx.Commit()
			c.JSON(http.StatusOK, gin.H{
				"code":    "0",
				"message": "quick_pass",
				"data": gin.H{
					"file_id": existingFile.FileID,
					"url":     existingFile.StoragePath,
				},
			})
			return
		}

		if err != gorm.ErrRecordNotFound {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error", "details": err.Error()})
			return
		}

		// ================= 情况 B: 文件不存在 (需要真正上传) =================

		// 2. 初始化 S3 分片上传
		s3Client, err := newS3Client(ctx)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "s3 client init failed"})
			return
		}

		// 生成 S3 Key (例如: files/ab/abcd1234...)
		s3Key := fmt.Sprintf("files/%s/%s", req.FileHash[:2], req.FileHash)

		// 调用 S3 CreateMultipartUpload
		createMultipartInput := &s3.CreateMultipartUploadInput{
			Bucket:      aws.String("your-bucket-name"), // TODO: load from config
			Key:         aws.String(s3Key),
			ContentType: aws.String("application/octet-stream"),
		}

		createOut, err := s3Client.CreateMultipartUpload(ctx, createMultipartInput)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "s3 multipart init failed", "details": err.Error()})
			return
		}
		uploadID := *createOut.UploadId

		// 3. 创建 FileMeta 记录 (状态设为 Uploading，等上传完成后再改为 Active)
		// 注意：此时还不创建 UserFileRelation，也不增加 ReferenceCount
		newFileMeta, err := createFileMeta(ctx, tx, req, s3Key)
		if err != nil {
			tx.Rollback()
			// 可选：调用 S3 AbortMultipartUpload 清理残留
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to create file meta"})
			return
		}

		// 4. 【关键】将 UploadID 与 用户/文件ID 绑定到 Redis
		// 格式：upload:{uploadID} -> "{userID}:{fileMetaID}"
		// 过期时间设为 25 小时，防止僵尸任务
		redisKey := fmt.Sprintf("upload:%s", uploadID)
		redisValue := fmt.Sprintf("%d:%d:%s:%s:%d", userID, newFileMeta.FileID, s3Key, req.FileName, req.ParentFolderID)

		if err := redisClient.Set(ctx, redisKey, redisValue, 25*time.Hour).Err(); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to bind upload ID in Redis"})
			return
		}
		tx.Commit()

		// 返回 UploadID 给前端，让前端开始分片上传
		c.JSON(http.StatusOK, gin.H{
			"code":    "0",
			"message": "init_success",
			"data": gin.H{
				"upload_id":  uploadID,
				"file_id":    newFileMeta.FileID,
				"s3_key":     s3Key,
				"chunk_size": ChunkSize, // 建议前端分片大小 5MB
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

		// 从 Redis 获取 UploadID 关联的信息
		redisKey := fmt.Sprintf("upload:%s", req.UploadID)
		redisValue, err := redisClient.Get(ctx, redisKey).Result()
		if err == redis.Nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid upload ID"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to query Redis"})
			return
		}

		// 解析 Redis 中的值，获取 userID、fileMetaID 和 s3Key(storagePath)
		var storedUserID uint
		var fileMetaID uint
		var s3Key string
		var fileName string
		var parentFolderID int
		_, err = fmt.Sscanf(redisValue, "%d:%d:%s:%s:%d", &storedUserID, &fileMetaID, &s3Key, &fileName, &parentFolderID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "invalid Redis value format"})
			return
		}

		// 验证用户 ID 是否匹配，防止越权访问
		if storedUserID != userID {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "unauthorized"})
			return
		}

		// 初始化 S3 客户端,获取分片上传的预签名 URL
		s3Client, err := newS3Client(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "s3 client init failed"})
			return
		}

		presignClient := s3.NewPresignClient(s3Client)
		presignExpires := 15 * time.Minute // 预签名 URL 有效期

		presignResult, err := presignClient.PresignUploadPart(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(""), //TODO: load from config
			Key:        aws.String(s3Key),
			PartNumber: &req.PartNumber,
			UploadId:   aws.String(req.UploadID),
		}, s3.WithPresignExpires(presignExpires))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to generate presigned URL", "details": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    "0",
			"message": "presign success",
			"data": gin.H{
				"presigned_url": presignResult.URL,
				"part_number":   req.PartNumber,
				"upload_id":     req.UploadID,
				"expires_in":    presignExpires.Seconds(),
				"http_method":   "PUT",
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

		// 从 Redis 获取 UploadID 关联的信息
		redisKey := fmt.Sprintf("upload:%s", req.UploadID)
		redisValue, err := redisClient.Get(ctx, redisKey).Result()
		if err == redis.Nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "upload task not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to query Redis", "details": err.Error()})
			return
		}

		// 解析 Redis 中的值，获取 userID、fileMetaID 和 s3Key(storagePath)
		var storedUserID uint
		var fileMetaID uint
		var s3Key string
		var fileName string
		var parentFolderID uint
		_, err = fmt.Sscanf(redisValue, "%d:%d:%s:%s:%d", &storedUserID, &fileMetaID, &s3Key, &fileName, &parentFolderID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "invalid Redis value format"})
			return
		}

		// 验证用户 ID 是否匹配，防止越权访问
		if storedUserID != userID {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "unauthorized"})
			return
		}

		s3Client, err := newS3Client(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "s3 client init failed"})
			return
		}

		// 构建 CompleteMultipartUploadInput, 开始事务
		tx := fileConnection.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()

		//数据库预检验文件状态，确保是 Uploading 状态
		var fileMeta models.FileMeta
		if err := tx.WithContext(ctx).First(&fileMeta, fileMetaID).Error; err != nil {
			tx.Rollback()
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"code": "1", "error": "file meta not found"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error", "details": err.Error()})
			}
			return
		}

		if fileMeta.Status != UploadStatusUploading {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "file is not in uploading state"})
			return
		}

		// 二次校验文件哈希，防止客户端篡改 UploadID 进行恶意请求
		if fileMeta.FileHash != req.FileHash {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "file hash mismatch"})
			return
		}

		//转换分片信息为 S3 需要的格式
		completeParts := make([]types.CompletedPart, len(req.Parts))
		for i, part := range req.Parts {
			completeParts[i] = types.CompletedPart{
				PartNumber: aws.Int32(part.PartNumber),
				ETag:       aws.String(part.ETag),
			}
		}

		//调用 s3 完成合并
		completeInput := &s3.CompleteMultipartUploadInput{
			Bucket:   aws.String("your-bucket-name"), //TODO: load from config
			Key:      aws.String(s3Key),
			UploadId: aws.String(req.UploadID),
			MultipartUpload: &types.CompletedMultipartUpload{
				Parts: completeParts,
			},
		}
		_, err = s3Client.CompleteMultipartUpload(ctx, completeInput)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to complete multipart upload", "details": err.Error()})
			return
		}

		// 更新文件元信息状态为 Active
		if err := tx.Model(&fileMeta).Updates(map[string]interface{}{
			"status": UploadStatusActive,
		}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to update file meta status"})
			return
		}

		userFile := models.UserFileRelation{
			UserID:         userID,
			FileMetaID:     fileMetaID,
			FileName:       fileName,
			ParentFolderID: parentFolderID,
			IsDeleted:      false,
		}
		if err := tx.Create(&userFile).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to create user file relation"})
			return
		}

		// 事务提交后删除 Redis 中的 UploadID 绑定，清理资源
		if err := tx.Commit().Error; err != nil {
			log.Printf("transaction commit failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "transaction commit failed", "details": err.Error()})
			return
		}

		_ = redisClient.Del(ctx, redisKey).Err() // 删除 Redis 键，清理资源

		c.JSON(http.StatusOK, gin.H{
			"code":    "0",
			"message": "upload complete",
			"data": gin.H{
				"file_id":   fileMetaID,
				"url":       fileMeta.StoragePath,
				"file_hash": fileMeta.FileHash,
			},
		})
	}
}
