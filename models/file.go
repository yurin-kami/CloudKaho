package models

import "time"

type FileMeta struct {
	FileID         uint   `gorm:"primaryKey" json:"file_id"`
	FileHash       string `gorm:"not null;unique;index" json:"file_hash"`
	FileSize       int64  `gorm:"not null" json:"file_size"`
	StoragePath    string `gorm:"not null" json:"storage_path"` //minio or oss
	ReferenceCount int    `gorm:"not null" json:"reference_count"`
	CreatedAt      int64  `gorm:"autoCreateTime" json:"created_at"`
	Status         int    `gorm:"not null;index" json:"status"` // 1=Active, 2=Uploading, 3=Deleted
}

type UserFileRelation struct {
	RelationID     uint   `gorm:"primaryKey" json:"relation_id"`
	UserID         uint   `gorm:"not null;index" json:"user_id"`
	FileName       string `gorm:"not null" json:"file_name"`
	ParentFolderID uint   `gorm:"not null;index" json:"parent_folder_id"`
	FileMetaID     uint   `gorm:"not null;foreignkey:FileID" json:"file_id"`
	IsDeleted      bool   `gorm:"not null;index" json:"is_deleted"`
	CreatedAt      int64  `gorm:"autoCreateTime" json:"created_at"`
}

type FileShare struct {
	ShareID      uint      `gorm:"primaryKey" json:"share_id"`
	FileID       uint      `gorm:"not null;index" json:"file_id"`
	ShareCode    string    `gorm:"not null;unique;index" json:"share_code"`
	UserID       uint      `gorm:"not null;index" json:"user_id"`
	FolderID     uint      `gorm:"not null;index" json:"folder_id"`
	ShareType    int       `gorm:"not null" json:"share_type"` // 1=encrypted, 2=public
	Password     string    `json:"password,omitempty"`         // 加密分享的密码，公开分享不需要
	ExpireAt     time.Time `gorm:"omitempty;index" json:"expire_at"`
	MaxViews     int       `gorm:"not null" json:"max_views"`     // 最大访问次数，0表示无限制
	CurrentViews int       `gorm:"not null" json:"current_views"` // 当前访问次数
	Status       int       `gorm:"not null;index" json:"status"`  // 1=Active, 2=Expired, 0=cancelled, 3=Deleted
	CreatedAt    int64     `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    int64     `gorm:"autoUpdateTime" json:"updated_at"`
}
