package file

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/service"
	"gorm.io/gorm"
)

type RenameRequest struct {
	FileID   uint   `json:"file_id" validate:"required"`
	NewName  string `json:"new_name" validate:"required"`
	FolderID uint   `json:"folder_id" validate:"required"`
}

type MoveRequest struct {
	FileID   uint `json:"file_id" validate:"required"`
	FolderID uint `json:"folder_id" validate:"required"`
}

func RenameFile(fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		var req RenameRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "Invalid request", "details": err.Error()})
			return
		}

		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "Unauthorized"})
			return
		}

		if err := service.RenameFileForUser(ctx, fileConnection, userID, req.FileID, req.NewName, req.FolderID); err != nil {
			if errors.Is(err, service.ErrConflict) {
				c.JSON(http.StatusConflict, gin.H{"code": "1", "error": "file with the same name already exists in the target folder"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error"})
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
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "Invalid request", "details": err.Error()})
			return
		}

		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "Unauthorized"})
			return
		}

		if err := service.MoveFileForUser(ctx, fileConnection, userID, req.FileID, req.FolderID); err != nil {
			if errors.Is(err, service.ErrConflict) {
				c.JSON(http.StatusConflict, gin.H{"code": "1", "error": "file with the same name already exists in the target folder"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error"})
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
