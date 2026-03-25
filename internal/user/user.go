package user

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/config"
	"github.com/yurin-kami/CloudKaho/internal/utils"
	"github.com/yurin-kami/CloudKaho/models"
	"github.com/yurin-kami/CloudKaho/service"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var userConnection *gorm.DB

func init() {
	var err error
	cfg := config.MustLoad()
	userConnection, err = gorm.Open(mysql.Open(cfg.DB.DSN), &gorm.Config{})
	if err != nil {
		panic(err)
	}
}

func hashPassword(password string) string {
	sha256Hasher := sha256.New()
	sha256Hasher.Write([]byte(password))
	return string(sha256Hasher.Sum(nil))
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	Nickname string `json:"nickname" binding:"required"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func UserRegister() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req registerRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		_, err := service.FindUserByEmail(ctx, userConnection, req.Email)
		if err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check user"})
			return
		}

		newUser := models.User{
			Email:          req.Email,
			HashedPassword: hashPassword(req.Password),
			Nickname:       req.Nickname,
		}
		if err := service.CreateUser(ctx, userConnection, &newUser); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to register user"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"code": "0",
			"user": newUser,
		})
	}
}

func UserLogin() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		user, err := service.FindUserByEmailAndPassword(ctx, userConnection, req.Email, hashPassword(req.Password))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "invalid credentials"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to login"})
			return
		}
		accessToken, err := utils.GenerateAccessToken(user, "access", time.Hour*25)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to generate access token"})
			return
		}
		refreshToken, err := utils.GenerateAccessToken(user, "refresh", time.Hour*24*7)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to generate refresh token"})
			return
		}

		user.AccessToken = accessToken
		user.RefreshToken = refreshToken
		if err := service.UpdateUserTokens(ctx, userConnection, &user); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to update user tokens"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":          "0",
			"user":          user,
			"access_token":  accessToken,
			"refresh_token": refreshToken,
		})
	}
}

// 通过token获取用户信息
func GetUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		userIDValue, exists := c.Get("userID")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":  "1",
				"error": "unauthorized",
			})
			return
		}
		userID, ok := userIDValue.(uint)
		if !ok || userID == 0 {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":  "1",
				"error": "unauthorized",
			})
			return
		}

		user, err := service.FindUserByID(ctx, userConnection, userID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": "1", "error": "user not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to fetch user"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code": "0",
			"user": user,
		})
	}
}
