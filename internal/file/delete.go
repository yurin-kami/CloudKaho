package file

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/models"
	"gorm.io/gorm"
)

type DeleteRequest struct {
	FileID uint `json:"file_id" validate:"required"`
}

func DeleteFile(fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		tx := fileConnection.WithContext(ctx).Begin()
		var req DeleteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "Invalid request", "details": err.Error()})
			return
		}

		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "Unauthorized"})
			return
		}
		if userID == 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "Unauthorized"})
			return
		}
		//检查用户是否有权限删除这个文件
		var relation models.UserFileRelation
		if err := fileConnection.WithContext(ctx).Where("user_id = ? AND file_meta_id = ?", userID, req.FileID).First(&relation).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusForbidden, gin.H{"code": "1", "error": "file not found or no permission"})
				tx.Rollback()
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error"})
			tx.Rollback()
			return
		}
		//标记用户文件关系为删除
		var userFile models.UserFileRelation
		if err := tx.Model(&userFile).WithContext(ctx).Where("user_id = ? AND file_id = ?", userID, req.FileID).Update("is_deleted", true).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error"})
			tx.Rollback()
			return
		}

		//减少文件元的引用计数
		var fileMeta models.FileMeta
		if err := tx.Model(&fileMeta).WithContext(ctx).Where("file_id = ?", req.FileID).First(&fileMeta).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error"})
			tx.Rollback()
			return
		}
		if fileMeta.ReferenceCount > 0 {
			if err := tx.Model(&fileMeta).WithContext(ctx).Where("file_id = ?", req.FileID).Update("reference_count", fileMeta.ReferenceCount-1).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error"})
				tx.Rollback()
				return
			}
		}
		tx.Commit()
		c.JSON(http.StatusOK, gin.H{"code": "0", "data": gin.H{
			"file_id": req.FileID,
			"message": "file marked as deleted successfully",
		}})
		return
		//真删除在cronjob中
	}
}
