package file

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/service"
	"gorm.io/gorm"
)

func writeSentinelError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrInvalid) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request", "details": err.Error()})
		return
	}

	if errors.Is(err, service.ErrForbidden) {
		c.JSON(http.StatusForbidden, gin.H{"code": "1", "error": "forbidden", "details": err.Error()})
		return
	}

	if errors.Is(err, service.ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"code": "1", "error": "not found", "details": err.Error()})
		return
	}

	if errors.Is(err, service.ErrConflict) {
		c.JSON(http.StatusConflict, gin.H{"code": "1", "error": "conflict", "details": err.Error()})
		return
	}

	c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "internal error"})
}
