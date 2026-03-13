package file

import (
	"context"
	"crypto/sha256"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
)

const (
	multipartThreshold = 16 * 1024 * 1024
	partSize           = 8 * 1024 * 1024
)

type preCheckRequest struct {
	FileHash       string `json:"file_hash" binding:"required"`
	FileSize       int64  `json:"file_size" binding:"required"`
	FileName       string `json:"file_name" binding:"required"`
	ParentFolderID uint   `json:"parent_folder_id"`
}

type completeUploadRequest struct {
	FileHash       string          `json:"file_hash" binding:"required"`
	FileSize       int64           `json:"file_size" binding:"required"`
	FileName       string          `json:"file_name" binding:"required"`
	ParentFolderID uint            `json:"parent_folder_id"`
	Parts          []completedPart `json:"part_id" binding:"required"`
}

type completedPart struct {
	PartNumber int32  `json:"part_number"`
	ETag       string `json:"etag"`
}

// TODO: 完成s3客户端从配置文件或环境变量加载的功能
func newS3Client(ctx context.Context) (*s3.Client, error) {
	return s3.NewFromConfig(aws.Config{}), nil
}

func InitMultipartUpload() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req preCheckRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request"})
			return
		}
	}
}

func UploadFile() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		userID, ok := c.Get("userID")
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":  "1",
				"error": "unauthorized",
			})
			return
		}

		// 解析请求参数,并且判断文件大小是否超过阈值
		var req preCheckRequest
		if err := c.ShouldBind(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request"})
			return
		}
		if req.FileSize > multipartThreshold {
			//
		}
		fileHeader, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "missing file"})
			return
		}

		src, err := fileHeader.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to open file"})
			return
		}
		defer src.Close()

		//sha256sum
		hasher := sha256.New()
		tee := io.TeeReader(src, hasher)

		s3Client, err := newS3Client(ctx)
		if err != nil {
			log.Printf("failed to create S3 client: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to create S3 client"})
			return
		}

		_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String("kaho-stream"),
			Key:         aws.String("load in config or env"),
			Body:        tee,
			ContentType: aws.String(fileHeader.Header.Get("Content-Type")),
		})
		if err != nil {
			log.Printf("failed to upload file: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to upload file"})
			return
		}
	}
}
