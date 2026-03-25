package utils

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/yurin-kami/CloudKaho/config"
	"github.com/yurin-kami/CloudKaho/models"
)

func GetJWTSecret() string {
	return config.Get().JWT.Secret
}

func GenerateAccessToken(user models.User, tokenType string, expire time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"type":  tokenType,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(expire).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(GetJWTSecret()))
}
