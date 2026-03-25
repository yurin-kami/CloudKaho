package service

import (
	"context"

	"github.com/yurin-kami/CloudKaho/models"
	"gorm.io/gorm"
)

func FindUserByEmail(ctx context.Context, db *gorm.DB, email string) (models.User, error) {
	var user models.User
	err := db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	return user, err
}

func CreateUser(ctx context.Context, db *gorm.DB, user *models.User) error {
	return db.WithContext(ctx).Create(user).Error
}

func FindUserByEmailAndPassword(ctx context.Context, db *gorm.DB, email string, hashedPassword string) (models.User, error) {
	var user models.User
	err := db.WithContext(ctx).Where("email = ? AND hashed_password = ?", email, hashedPassword).First(&user).Error
	return user, err
}

func UpdateUserTokens(ctx context.Context, db *gorm.DB, user *models.User) error {
	return db.WithContext(ctx).Save(user).Error
}

func FindUserByID(ctx context.Context, db *gorm.DB, userID uint) (models.User, error) {
	var user models.User
	err := db.WithContext(ctx).Where("id = ?", userID).First(&user).Error
	return user, err
}
