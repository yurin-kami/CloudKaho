package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/internal/middleware"
	"github.com/yurin-kami/CloudKaho/internal/user"
)

func UserRoute(router *gin.Engine) {
	userGroup := router.Group("/user")
	userGroup.POST("/register", user.UserRegister())
	userGroup.POST("/login", user.UserLogin())
	userGroup.GET("/me", middleware.AuthRequired(), user.GetUser())
}
