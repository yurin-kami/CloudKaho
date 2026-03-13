package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/internal/file"
	"github.com/yurin-kami/CloudKaho/internal/middleware"
)

func FileRoute(router *gin.Engine) {
	fileGroup := router.Group("/file")
	fileGroup.POST("/upload", middleware.AuthRequired(), file.UploadFile())
	fileGroup.GET("/download/:file_id", middleware.AuthRequired(), file.DownloadFile())
	fileGroup.POST("/delete", middleware.AuthRequired(), file.DeleteFile())
	fileGroup.PUT("/rename", middleware.AuthRequired(), file.RenameFile())
	fileGroup.POST("/move", middleware.AuthRequired(), file.MoveFile())
	fileGroup.GET("/list/:folder_id", middleware.AuthRequired(), file.ListFiles())
	fileGroup.GET("/share/:file_id", middleware.AuthRequired(), file.ShareFile())
}
