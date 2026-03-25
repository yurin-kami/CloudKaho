package file

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/models"
	"github.com/yurin-kami/CloudKaho/service"
	"gorm.io/gorm"
)

type ShareRequest struct {
	FileID    uint   `json:"file_id" validate:"required"`
	ShareType int    `json:"share_type" validate:"required,oneof=1 2"` // 1=encrypted, 2=public
	Password  string `json:"password,omitempty"`                       // 加密分享的密码，公开分享不需要
	ExpireAt  int64  `json:"expire_at,omitempty"`                      // 过期时间，单位为秒，0表示永不过期
	MaxViews  int    `json:"max_views,omitempty"`                      // 最大访问次数，0表示无限制
}

func ListFiles(fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		// 获取用户ID
		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "Unauthorized"})
			return
		}

		folderID := c.Param("folder_id")

		userFiles, err := service.ListFilesForUser(ctx, fileConnection, userID, folderID)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusOK, gin.H{"code": "0", "files": []models.UserFileRelation{}})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "Database error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": "0", "files": userFiles})
	}
}

func ShareFile(fileConnection *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		var req ShareRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "Invalid request", "details": err.Error()})
			return
		}

		// 获取用户ID
		userID, exists := getUserIDFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "Unauthorized"})
			return
		}

		shareURL, err := service.CreateShareAndGetURL(ctx, fileConnection, userID, req.FileID, req.ShareType, req.Password, req.ExpireAt, req.MaxViews)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				c.JSON(404, gin.H{"code": "1", "error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "Database error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"code": "0", "share_url": shareURL})
	}
}
