package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/yurin-kami/CloudKaho/internal/file"
	"github.com/yurin-kami/CloudKaho/internal/middleware"
	"gorm.io/gorm"
)

func FileRoute(router *gin.Engine, redisClient *redis.Client, fileConnection *gorm.DB) {
	fileGroup := router.Group("/file")
	// fileGroup.POST("/upload", middleware.AuthRequired(), file.UploadFile())
	uploadGroup := fileGroup.Group("/upload")
	multipartGroup := uploadGroup.Group("/multipart")
	uploadGroup.POST("/upload-small", middleware.AuthRequired(), file.UploadSmallFile(fileConnection))                     // 如果文件小于5MB，直接上传
	multipartGroup.POST("/init", middleware.AuthRequired(), file.InitMultipartUpload(redisClient, fileConnection))         // 初始化分片上传，返回UploadID
	multipartGroup.POST("/presign", middleware.AuthRequired(), file.PartPresignURL(redisClient, fileConnection))           // 获取分片上传的预签名URL
	multipartGroup.POST("/complete", middleware.AuthRequired(), file.CompleteMultipartUpload(redisClient, fileConnection)) // 完成分片上传，合并分片
	fileGroup.GET("/download/:file_id", middleware.AuthRequired(), file.DownloadFile(redisClient, fileConnection))
	fileGroup.POST("/delete", middleware.AuthRequired(), file.DeleteFile(fileConnection))
	fileGroup.PUT("/rename", middleware.AuthRequired(), file.RenameFile())
	fileGroup.POST("/move", middleware.AuthRequired(), file.MoveFile())
	fileGroup.GET("/list/:folder_id", middleware.AuthRequired(), file.ListFiles())
	fileGroup.GET("/share/:file_id", middleware.AuthRequired(), file.ShareFile())
}
