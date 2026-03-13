package utils

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/yurin-kami/CloudKaho/models"
)

const JWTSecret = "KahoKoyanagi"

func GenerateAccessToken(user models.User, tokenType string, expire time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"type":  tokenType,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(expire).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	//TODO: load secret from config or env
	return token.SignedString([]byte(JWTSecret))
}
