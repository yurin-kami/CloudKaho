package user

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/yurin-kami/CloudKaho/config"
	"github.com/yurin-kami/CloudKaho/internal/utils"
	"github.com/yurin-kami/CloudKaho/models"
	"github.com/yurin-kami/CloudKaho/service"
	"golang.org/x/crypto/bcrypt"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var userConnection *gorm.DB

var nonAlphaNumeric = regexp.MustCompile(`[^a-z0-9]+`)

func init() {
	var err error
	cfg := config.MustLoad()
	userConnection, err = gorm.Open(gormmysql.Open(cfg.DB.DSN), &gorm.Config{})
	if err != nil {
		panic(err)
	}
}

const bcryptCost = 12 // 2026 年推荐值

// hashPassword 使用 bcrypt 哈希密码 (新用户注册)
func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// verifyLegacySHA256 验证旧格式密码 (仅用于迁移)
func verifyLegacySHA256(password, storedHash string) bool {
	if len(storedHash) != 64 {
		return false
	}
	h := sha256.Sum256([]byte(password))
	computed := hex.EncodeToString(h[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}

// identifyHashType 识别密码哈希类型
func identifyHashType(hash string) string {
	if strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") {
		return "bcrypt"
	}
	if len(hash) == 64 {
		// 简单验证 hex 格式
		if _, err := hex.DecodeString(hash); err == nil {
			return "sha256-hex"
		}
	}
	return "unknown"
}

func sanitizeUsernameBase(email string) string {
	parts := strings.SplitN(email, "@", 2)
	base := strings.ToLower(strings.TrimSpace(parts[0]))
	base = nonAlphaNumeric.ReplaceAllString(base, "")
	if base == "" {
		return "user"
	}
	return base
}

func randomUsernameSuffix() (string, error) {
	raw := make([]byte, 3)
	if _, err := cryptorand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func generateCandidateUsername(email string) (string, error) {
	base := sanitizeUsernameBase(email)
	suffix, err := randomUsernameSuffix()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%s", base, suffix), nil
}

func isUniqueConstraintError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return false
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
			c.JSON(http.StatusBadRequest, gin.H{"code": "1", "error": "invalid request", "details": err.Error()})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		_, err := service.FindUserByEmail(ctx, userConnection, req.Email)
		if err == nil {
			c.JSON(http.StatusConflict, gin.H{"code": "1", "error": "email already registered"})
			return
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to check user", "details": err.Error()})
			return
		}

		const maxUsernameAttempts = 5
		var newUser models.User
		for i := 0; i < maxUsernameAttempts; i++ {
			username, genErr := generateCandidateUsername(req.Email)
			if genErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to generate username"})
				return
			}

			hashedPassword, hashErr := hashPassword(req.Password)
			if hashErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to hash password"})
				return
			}

			newUser = models.User{
				Username:       username,
				Email:          req.Email,
				HashedPassword: hashedPassword,
				Nickname:       req.Nickname,
			}

			err = service.CreateUser(ctx, userConnection, &newUser)
			if err == nil {
				break
			}
			if !isUniqueConstraintError(err) {
				c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to register user", "details": err.Error()})
				return
			}

			if _, findErr := service.FindUserByEmail(ctx, userConnection, req.Email); findErr == nil {
				c.JSON(http.StatusConflict, gin.H{"code": "1", "error": "email already registered"})
				return
			}
		}

		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"code": "1", "error": "failed to allocate unique username"})
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

		// 步骤1: 通过邮箱查找用户
		user, err := service.FindUserByEmail(ctx, userConnection, req.Email)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "invalid credentials"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to login", "details": err.Error()})
			return
		}

		// 步骤2: 识别哈希类型并验证
		hashType := identifyHashType(user.HashedPassword)

		var passwordValid bool
		var needsMigration bool

		switch hashType {
		case "bcrypt":
			// 新格式: 直接验证
			err := bcrypt.CompareHashAndPassword([]byte(user.HashedPassword), []byte(req.Password))
			passwordValid = (err == nil)
			needsMigration = false

		case "sha256-hex":
			// 旧格式: 验证 + 标记需要迁移
			passwordValid = verifyLegacySHA256(req.Password, user.HashedPassword)
			needsMigration = true

		default:
			log.Printf("unknown hash type for user %d", user.ID)
			passwordValid = false
		}

		if !passwordValid {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "1", "error": "invalid credentials"})
			return
		}

		// 步骤3: 自动迁移旧密码 (异步执行,不阻塞登录)
		if needsMigration {
			go func() {
				migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer migrateCancel()

				newHash, hashErr := hashPassword(req.Password)
				if hashErr != nil {
					log.Printf("failed to generate bcrypt hash for user %d: %v", user.ID, hashErr)
					return
				}

				updateErr := service.UpdateUserPasswordHash(migrateCtx, userConnection, user.ID, newHash)
				if updateErr != nil {
					log.Printf("failed to migrate password for user %d: %v", user.ID, updateErr)
				} else {
					log.Printf("password migrated to bcrypt for user %d", user.ID)
				}
			}()
		}

		// 步骤4: 生成 JWT tokens (原有逻辑)
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
			c.JSON(http.StatusInternalServerError, gin.H{"code": "1", "error": "failed to fetch user", "details": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code": "0",
			"user": user,
		})
	}
}
