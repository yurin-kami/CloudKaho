package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/internal/file"
	"github.com/yurin-kami/CloudKaho/internal/middleware"
)

func FileRoute(router *gin.Engine) {
	fileGroup := router.Group("/file")
	// fileGroup.POST("/upload", middleware.AuthRequired(), file.UploadFile())
	uploadGroup := fileGroup.Group("/upload")
	multipartGroup := uploadGroup.Group("/multipart")
	uploadGroup.POST("/upload-small", middleware.AuthRequired(), file.UploadSmallFile())        // 如果文件小于5MB，直接上传
	multipartGroup.POST("/init", middleware.AuthRequired(), file.InitMultipartUpload())         // 初始化分片上传，返回UploadID
	multipartGroup.POST("/presign", middleware.AuthRequired(), file.PartPresignURL())           // 获取分片上传的预签名URL
	multipartGroup.POST("/complete", middleware.AuthRequired(), file.CompleteMultipartUpload()) // 完成分片上传，合并分片
	multipartGroup.DELETE("/:uploadId", middleware.AuthRequired(), file.CancelUpload())         // 取消分片上传，删除已上传的分片
	fileGroup.GET("/download/:file_id", middleware.AuthRequired(), file.DownloadFile())
	fileGroup.POST("/delete", middleware.AuthRequired(), file.DeleteFile())
	fileGroup.PUT("/rename", middleware.AuthRequired(), file.RenameFile())
	fileGroup.POST("/move", middleware.AuthRequired(), file.MoveFile())
	fileGroup.GET("/list/:folder_id", middleware.AuthRequired(), file.ListFiles())
	fileGroup.GET("/share/:file_id", middleware.AuthRequired(), file.ShareFile())
}
