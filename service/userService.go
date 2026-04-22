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

// FindUserByEmailAndPassword - 已废弃,迁移到 bcrypt 后不再使用
// 保留此注释以说明旧函数已被移除

func UpdateUserTokens(ctx context.Context, db *gorm.DB, user *models.User) error {
	return db.WithContext(ctx).Save(user).Error
}

func FindUserByID(ctx context.Context, db *gorm.DB, userID uint) (models.User, error) {
	var user models.User
	err := db.WithContext(ctx).Where("id = ?", userID).First(&user).Error
	return user, err
}

// UpdateUserPasswordHash 更新用户密码哈希 (用于迁移)
func UpdateUserPasswordHash(ctx context.Context, db *gorm.DB, userID uint, newHash string) error {
	return db.WithContext(ctx).Model(&models.User{}).
		Where("id = ?", userID).
		Update("hashed_password", newHash).Error
}
