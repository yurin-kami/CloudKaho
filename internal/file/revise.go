package file

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RenameRequest struct {
	FileID   uint   `json:"file_id" validate:"required"`
	NewName  string `json:"new_name" validate:"required"`
	FolderID uint   `json:"folder_id" validate:"required"`
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

		tx := fileConnection.WithContext(ctx).Begin()
		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "Unauthorized"})
			tx.Rollback()
			return
		}
		//执行加锁事务
		var count int64
		if err := tx.WithContext(ctx).Where("user_id = ? AND file_id = ? AND parent_folder_id != ? AND file_name = ?", userID, req.FileID, req.FolderID, req.NewName).Clauses(clause.Locking{Strength: "UPDATE"}).Model(&models.UserFileRelation{}).Count(&count).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error"})
			tx.Rollback()
			return
		}
		if count > 0 {
			c.JSON(http.StatusConflict, gin.H{"code": "1", "error": "file with the same name already exists in the target folder"})
			tx.Rollback()
			return
		}

		//更新文件名和父文件夹ID
		if err := tx.WithContext(ctx).Model(&models.UserFileRelation{}).
			Where("user_id = ? AND file_id = ?", userID, req.FileID).Update("file_name", req.NewName).Update("parent_folder_id", req.FolderID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error"})
			tx.Rollback()
			return
		}
		tx.Commit()

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
