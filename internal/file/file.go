package file

import (
	"context"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const (
	multipartThreshold = 5 * 1024 * 1024
	partSize           = 5 * 1024 * 1024
)

var fileConnection *gorm.DB

func init() {
	var err error
	//TODO: 从配置文件或环境变量加载数据库连接信息
	fileConnection, err = gorm.Open(mysql.Open("root:kami@tcp(localhost:3306)/cloud-kaho?charset=utf8mb4&parseTime=True"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
}

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
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var req preCheckRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request"})
			return
		}

		var existingFile models.FileMeta
		if err := fileConnection.WithContext(ctx).Where("file_hash = ?", req.FileHash).First(&existingFile).Error; err != nil {

			if err == gorm.ErrRecordNotFound { //上传到s3,存入FileMeta表
				insertFileMeta := models.FileMeta{
					FileHash:       existingFile.FileHash,
					FileSize:       existingFile.FileSize,
					ReferenceCount: 1,
					StoragePath:    "", //TODO use s3 to upload and get url
				}

				if insertErr := fileConnection.Create(&insertFileMeta).Error; insertErr != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "file not found in s3 and upload failed"})
					return
				}

				//成功后在UserFileRelation表添加记录
				if queryErr := fileConnection.WithContext(ctx).First(&existingFile).Error; queryErr != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "FileMeta data error"})
					return
				}

				userIDValue, exists := c.Get("userID")
				if !exists {
					c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "unauthorized"})
					return
				}

				userID, ok := userIDValue.(uint)
				if !ok {
					c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "unauthorized"})
					return
				}

				newUserFile := models.UserFileRelation{
					UserID:         userID,
					FileName:       req.FileName,
					ParentFolderID: req.ParentFolderID,
					FileMetaID:     existingFile.FileID,
					IsDeleted:      false,
				}
				if insertUserFileErr := fileConnection.WithContext(ctx).Create(&newUserFile).Error; insertUserFileErr != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "insert user file error"})
					return
				}
			}
		}

	}
}
