package models

type User struct {
	ID             uint    `gorm:"primaryKey" json:"id"`
	Username       string  `gorm:"unique;not null" json:"username"`
	Email          string  `gorm:"unique;not null" json:"email"`
	HashedPassword string  `gorm:"not null" json:"-"`
	Nickname       string  `gorm:"not null" json:"nickname"`
	Total          float32 `gorm:"default:10;not null" json:"total"`
	Usage          float32 `gorm:"default:0;not null" json:"usage"`
	AccessToken    string  `gorm:"" json:"access_token"`
	RefreshToken   string  `gorm:"" json:"refresh_token"`
	CreatedAt      int64   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      int64   `gorm:"autoUpdateTime" json:"updated_at"`
}
