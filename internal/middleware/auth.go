package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/yurin-kami/CloudKaho/internal/utils"
)

// token解析和验证中间件
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		bearer := c.GetHeader("Authorization")
		if bearer == "" || !strings.HasPrefix(bearer, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":  "1",
				"error": "missing or invalid authorization header",
			})
			return
		}

		tokenStr := strings.TrimPrefix(bearer, "Bearer ")
		claims := jwt.MapClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(utils.JWTSecret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":  "1",
				"error": "invalid token",
			})
			return
		}

		tokenType, ok := claims["type"].(string)
		if !ok || tokenType != "access" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":  "1",
				"error": "invalid token type",
			})
			return
		}

		sub, ok := claims["sub"]
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":  "1",
				"error": "invalid token",
			})
			return
		}

		var userID uint
		switch value := sub.(type) {
		case float64:
			if value <= 0 {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"code":  "1",
					"error": "invalid token",
				})
				return
			}
			userID = uint(value)
		case string:
			parsedID, parseErr := strconv.ParseUint(value, 10, 64)
			if parseErr != nil || parsedID == 0 {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"code":  "1",
					"error": "invalid token",
				})
				return
			}
			userID = uint(parsedID)
		default:
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":  "1",
				"error": "invalid token",
			})
			return
		}

		c.Set("userID", userID)
		c.Next()
	}
}
