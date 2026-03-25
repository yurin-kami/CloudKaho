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

type DeleteRequest struct {
	FileID uint `json:"file_id" validate:"required"`
}

func DeleteFile(fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

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

		if err := service.DeleteFileForUser(ctx, fileConnection, userID, req.FileID); err != nil {
			if errors.Is(err, service.ErrForbidden) || errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusForbidden, gin.H{"code": "1", "error": "file not found or no permission"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "database error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": "0", "data": gin.H{
			"file_id": req.FileID,
			"message": "file marked as deleted successfully",
		}})
		return
		//真删除在cronjob中
	}
}
