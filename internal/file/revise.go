package file

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/service"
	"gorm.io/gorm"
)

type RenameRequest struct {
	FileID   uint   `json:"file_id" binding:"required,gt=0"`
	NewName  string `json:"new_name" binding:"required"`
	FolderID uint   `json:"folder_id" binding:"required,gt=0"`
}

type MoveRequest struct {
	FileID   uint `json:"file_id" binding:"required,gt=0"`
	FolderID uint `json:"folder_id" binding:"required,gt=0"`
}

func RenameFile(fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		var req RenameRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request", "details": err.Error()})
			return
		}
		req.NewName = strings.TrimSpace(req.NewName)
		if req.NewName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request", "details": "new_name cannot be empty"})
			return
		}

		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "unauthorized"})
			return
		}

		if err := service.RenameFileForUser(ctx, fileConnection, userID, req.FileID, req.NewName, req.FolderID); err != nil {
			writeSentinelError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code": "0",
			"data": gin.H{
				"file_id":   req.FileID,
				"new_name":  req.NewName,
				"folder_id": req.FolderID,
			},
		})
	}
}

func MoveFile(fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		var req MoveRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request", "details": err.Error()})
			return
		}

		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "unauthorized"})
			return
		}

		if err := service.MoveFileForUser(ctx, fileConnection, userID, req.FileID, req.FolderID); err != nil {
			writeSentinelError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code": "0",
			"data": gin.H{
				"file_id":   req.FileID,
				"folder_id": req.FolderID,
			},
		})
	}
}
