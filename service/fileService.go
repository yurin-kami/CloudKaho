package service

import (
	"context"
	"time"

	"github.com/yurin-kami/CloudKaho/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func FindActiveFileByHash(ctx context.Context, db *gorm.DB, fileHash string, activeStatus int) (*models.FileMeta, error) {
	var existingFile models.FileMeta
	err := db.WithContext(ctx).
		Where("file_hash = ? AND status = ?", fileHash, activeStatus).
		First(&existingFile).Error
	if err != nil {
		return nil, err
	}
	return &existingFile, nil
}

func FindActiveFileByHashForUpdate(ctx context.Context, db *gorm.DB, fileHash string, activeStatus int) (*models.FileMeta, error) {
	var existingFile models.FileMeta
	err := db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("file_hash = ? AND status = ?", fileHash, activeStatus).
		First(&existingFile).Error
	if err != nil {
		return nil, err
	}
	return &existingFile, nil
}

func IncrementReferenceCount(ctx context.Context, db *gorm.DB, fileMeta *models.FileMeta) error {
	return db.WithContext(ctx).
		Model(fileMeta).
		Update("reference_count", gorm.Expr("reference_count + ?", 1)).Error
}

func CreateUserFileRelation(ctx context.Context, db *gorm.DB, userID uint, fileMetaID uint, fileName string, parentFolderID uint) error {
	userFile := models.UserFileRelation{
		UserID:         userID,
		FileMetaID:     fileMetaID,
		FileName:       fileName,
		ParentFolderID: parentFolderID,
		IsDeleted:      false,
	}
	return db.WithContext(ctx).Create(&userFile).Error
}

func CreateFileMeta(ctx context.Context, db *gorm.DB, fileHash string, fileSize int64, storagePath string, status int, referenceCount int) (*models.FileMeta, error) {
	newFileMeta := models.FileMeta{
		FileHash:       fileHash,
		FileSize:       fileSize,
		ReferenceCount: referenceCount,
		StoragePath:    storagePath,
		Status:         status,
	}
	if err := db.WithContext(ctx).Create(&newFileMeta).Error; err != nil {
		return nil, err
	}
	return &newFileMeta, nil
}

func GetUserFileRelationByMetaID(ctx context.Context, db *gorm.DB, userID uint, fileMetaID uint, includeDeleted bool) (models.UserFileRelation, error) {
	var relation models.UserFileRelation
	query := db.WithContext(ctx).Where("user_id = ? AND file_meta_id = ?", userID, fileMetaID)
	if !includeDeleted {
		query = query.Where("is_deleted = ?", false)
	}
	err := query.First(&relation).Error
	return relation, err
}

func GetUserFileRelationByFileID(ctx context.Context, db *gorm.DB, userID uint, fileID uint) (models.UserFileRelation, error) {
	var relation models.UserFileRelation
	err := db.WithContext(ctx).Where("user_id = ? AND file_id = ?", userID, fileID).First(&relation).Error
	return relation, err
}

func ListUserFilesByFolder(ctx context.Context, db *gorm.DB, userID uint, folderID string) ([]models.UserFileRelation, error) {
	var userFiles []models.UserFileRelation
	err := db.WithContext(ctx).Where("user_id = ? AND parent_folder_id = ?", userID, folderID).Find(&userFiles).Error
	return userFiles, err
}

func CountUserFileRenameConflict(ctx context.Context, db *gorm.DB, userID uint, fileID uint, folderID uint, fileName string) (int64, error) {
	var count int64
	err := db.WithContext(ctx).
		Where("user_id = ? AND file_id = ? AND parent_folder_id != ? AND file_name = ?", userID, fileID, folderID, fileName).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Model(&models.UserFileRelation{}).
		Count(&count).Error
	return count, err
}

func CountUserFileMoveConflict(ctx context.Context, db *gorm.DB, userID uint, fileID uint, folderID uint) (int64, error) {
	var count int64
	err := db.WithContext(ctx).
		Where("user_id = ? AND file_id = ? AND parent_folder_id != ?", userID, fileID, folderID).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Model(&models.UserFileRelation{}).
		Count(&count).Error
	return count, err
}

func UpdateUserFileNameAndFolder(ctx context.Context, db *gorm.DB, userID uint, fileID uint, fileName string, folderID uint) error {
	return db.WithContext(ctx).Model(&models.UserFileRelation{}).
		Where("user_id = ? AND file_id = ?", userID, fileID).
		Update("file_name", fileName).Update("parent_folder_id", folderID).Error
}

func UpdateUserFileFolder(ctx context.Context, db *gorm.DB, userID uint, fileID uint, folderID uint) error {
	return db.WithContext(ctx).Model(&models.UserFileRelation{}).
		Where("user_id = ? AND file_id = ?", userID, fileID).
		Update("parent_folder_id", folderID).Error
}

func MarkUserFileDeleted(ctx context.Context, db *gorm.DB, userID uint, fileID uint) error {
	return db.WithContext(ctx).Model(&models.UserFileRelation{}).
		Where("user_id = ? AND file_id = ?", userID, fileID).
		Update("is_deleted", true).Error
}

func GetFileMetaByID(ctx context.Context, db *gorm.DB, fileID uint) (models.FileMeta, error) {
	var fileMeta models.FileMeta
	err := db.WithContext(ctx).Where("file_id = ?", fileID).First(&fileMeta).Error
	return fileMeta, err
}

func GetFileMetaByIDForUpdate(ctx context.Context, db *gorm.DB, fileID uint) (models.FileMeta, error) {
	var fileMeta models.FileMeta
	err := db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("file_id = ?", fileID).
		First(&fileMeta).Error
	return fileMeta, err
}

func UpdateFileMetaReferenceCount(ctx context.Context, db *gorm.DB, fileID uint, referenceCount int) error {
	return db.WithContext(ctx).Model(&models.FileMeta{}).
		Where("file_id = ?", fileID).
		Update("reference_count", referenceCount).Error
}

func DecrementFileMetaReferenceCount(ctx context.Context, db *gorm.DB, fileID uint) error {
	return db.WithContext(ctx).Model(&models.FileMeta{}).
		Where("file_id = ? AND reference_count > 0", fileID).
		Update("reference_count", gorm.Expr("reference_count - 1")).Error
}

func UpdateFileMetaStatus(ctx context.Context, db *gorm.DB, fileMetaID uint, status int) error {
	return db.WithContext(ctx).Model(&models.FileMeta{}).
		Where("file_id = ?", fileMetaID).
		Updates(map[string]interface{}{"status": status}).Error
}

func CreateFileShare(ctx context.Context, db *gorm.DB, fileShare *models.FileShare) error {
	return db.WithContext(ctx).Clauses(clause.Locking{Strength: "Create"}).Create(fileShare).Error
}

func GetFileShareByIDForUpdate(ctx context.Context, db *gorm.DB, shareID uint) (models.FileShare, error) {
	var share models.FileShare
	err := db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("share_id = ?", shareID).
		First(&share).Error
	return share, err
}

func IncrementFileShareViews(ctx context.Context, db *gorm.DB, shareID uint) error {
	return db.WithContext(ctx).Model(&models.FileShare{}).
		Where("share_id = ?", shareID).
		Update("current_views", gorm.Expr("current_views + 1")).Error
}

func FindStaleUploads(ctx context.Context, db *gorm.DB, status int, thresholdTime time.Time, limit int) ([]models.FileMeta, error) {
	var staleFiles []models.FileMeta
	err := db.WithContext(ctx).Where("status = ? AND create_at < ?", status, thresholdTime).Limit(limit).Find(&staleFiles).Error
	return staleFiles, err
}
