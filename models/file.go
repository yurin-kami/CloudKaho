package models

type FileMeta struct {
	FileID         uint   `gorm:"primaryKey" json:"file_id"`
	FileHash       string `gorm:"not null;unique;index" json:"file_hash"`
	FileSize       int64  `gorm:"not null" json:"file_size"`
	StoragePath    string `gorm:"not null" json:"storage_path"` //minio or oss
	ReferenceCount int    `gorm:"not null" json:"reference_count"`
	CreatedAt      int64  `gorm:"autoCreateTime" json:"created_at"`
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
