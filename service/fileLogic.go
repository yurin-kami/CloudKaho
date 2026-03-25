package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/redis/go-redis/v9"
	appconfig "github.com/yurin-kami/CloudKaho/config"
	"github.com/yurin-kami/CloudKaho/models"
	"gorm.io/gorm"
)

const (
	UploadStatusActive    = 1
	UploadStatusUploading = 2
)

type UploadSmallFileResult struct {
	Message  string
	FileID   uint
	URL      string
	FileHash string
}

type InitMultipartResult struct {
	Message   string
	UploadID  string
	FileID    uint
	S3Key     string
	ChunkSize int64
	URL       string
}

type PresignPartResult struct {
	PresignedURL string
	ExpiresIn    float64
	HTTPMethod   string
}

type CompleteMultipartResult struct {
	FileID   uint
	URL      string
	FileHash string
}

func DeleteFileForUser(ctx context.Context, db *gorm.DB, userID uint, fileID uint) error {
	return WithTxRetry(ctx, db, 3, func(tx *gorm.DB) error {
		if _, err := GetUserFileRelationByMetaID(ctx, tx, userID, fileID, true); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: file not found or no permission", ErrForbidden)
			}
			return err
		}

		fileMeta, err := GetFileMetaByIDForUpdate(ctx, tx, fileID)
		if err != nil {
			return err
		}

		if err := MarkUserFileDeleted(ctx, tx, userID, fileID); err != nil {
			return err
		}

		if fileMeta.ReferenceCount > 0 {
			if err := DecrementFileMetaReferenceCount(ctx, tx, fileID); err != nil {
				return err
			}
		}

		return nil
	})
}

func GetDownloadURLForUser(ctx context.Context, db *gorm.DB, userID uint, fileID uint, fileName string) (string, time.Duration, error) {
	if _, err := GetUserFileRelationByMetaID(ctx, db, userID, fileID, false); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", 0, fmt.Errorf("%w: file not found or no permission", ErrForbidden)
		}
		return "", 0, err
	}

	fileMeta, err := GetFileMetaByID(ctx, db, fileID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", 0, fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return "", 0, err
	}
	if fileMeta.Status != UploadStatusActive {
		return "", 0, fmt.Errorf("%w: file not active", ErrInvalid)
	}

	s3Client, err := NewS3Client(ctx)
	if err != nil {
		return "", 0, err
	}

	presignClient := s3.NewPresignClient(s3Client)
	presignExpires := 15 * time.Minute
	s3Bucket := appconfig.Get().S3.Bucket
	presignResult, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(s3Bucket),
		Key:                        aws.String(fileMeta.StoragePath),
		ResponseContentDisposition: aws.String(fmt.Sprintf(`attachment; filename="%s"`, url.QueryEscape(fileName))),
	}, s3.WithPresignExpires(presignExpires))
	if err != nil {
		return "", 0, err
	}

	return presignResult.URL, presignExpires, nil
}

func CreateShareAndGetURL(ctx context.Context, db *gorm.DB, userID uint, fileID uint, shareType int, password string, expireAt int64, maxViews int) (string, error) {
	userFile, err := GetUserFileRelationByFileID(ctx, db, userID, fileID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return "", err
	}

	tx := db.WithContext(ctx).Begin()
	if err := tx.Error; err != nil {
		return "", err
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := CreateFileShare(ctx, tx, &models.FileShare{
		FileID:    fileID,
		UserID:    userID,
		ShareType: shareType,
		Password:  password,
		ExpireAt:  time.Unix(expireAt, 0),
		MaxViews:  maxViews,
		Status:    1,
	}); err != nil {
		tx.Rollback()
		return "", err
	}

	fileMeta, err := GetFileMetaByID(ctx, tx, userFile.FileMetaID)
	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("%w: file metadata not found", ErrNotFound)
		}
		return "", err
	}

	s3Client, err := NewS3Client(ctx)
	if err != nil {
		tx.Rollback()
		return "", err
	}
	presignedClient := s3.NewPresignClient(s3Client)
	s3Bucket := appconfig.Get().S3.Bucket
	presignResult, err := presignedClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(s3Bucket),
		Key:                        aws.String(fileMeta.FileHash),
		ResponseContentDisposition: aws.String(fmt.Sprintf("attachment; filename=\"%s\"", userFile.FileName)),
	}, s3.WithPresignExpires(time.Duration(expireAt-time.Now().Unix())*time.Second))
	if err != nil {
		tx.Rollback()
		return "", err
	}

	if err := tx.Commit().Error; err != nil {
		return "", err
	}

	return presignResult.URL, nil
}

func UploadSmallFile(ctx context.Context, db *gorm.DB, userID uint, fileSize int64, fileName string, parentFolderID uint, fileBytes []byte, contentType string) (UploadSmallFileResult, error) {
	fileHashBytes := sha256.Sum256(fileBytes)
	fileHash := hex.EncodeToString(fileHashBytes[:])

	tx := db.WithContext(ctx).Begin()
	if err := tx.Error; err != nil {
		return UploadSmallFileResult{}, err
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	existingFile, err := FindActiveFileByHashForUpdate(ctx, tx, fileHash, UploadStatusActive)
	if err == nil {
		if err := IncrementReferenceCount(ctx, tx, existingFile); err != nil {
			tx.Rollback()
			return UploadSmallFileResult{}, err
		}
		if err := CreateUserFileRelation(ctx, tx, userID, existingFile.FileID, fileName, parentFolderID); err != nil {
			tx.Rollback()
			return UploadSmallFileResult{}, err
		}
		if err := tx.Commit().Error; err != nil {
			return UploadSmallFileResult{}, err
		}
		return UploadSmallFileResult{
			Message:  "quick_pass",
			FileID:   existingFile.FileID,
			URL:      existingFile.StoragePath,
			FileHash: existingFile.FileHash,
		}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		tx.Rollback()
		return UploadSmallFileResult{}, err
	}

	s3Key := fmt.Sprintf("files/%s/%s", fileHash[:2], fileHash)
	s3Client, err := NewS3Client(ctx)
	if err != nil {
		tx.Rollback()
		return UploadSmallFileResult{}, err
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	s3Bucket := appconfig.Get().S3.Bucket
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s3Bucket),
		Key:           aws.String(s3Key),
		Body:          bytes.NewReader(fileBytes),
		ContentLength: aws.Int64(fileSize),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		tx.Rollback()
		return UploadSmallFileResult{}, err
	}

	newFileMeta, err := CreateFileMeta(ctx, tx, fileHash, fileSize, s3Key, UploadStatusActive, 1)
	if err != nil {
		tx.Rollback()
		return UploadSmallFileResult{}, err
	}
	if err := CreateUserFileRelation(ctx, tx, userID, newFileMeta.FileID, fileName, parentFolderID); err != nil {
		tx.Rollback()
		return UploadSmallFileResult{}, err
	}

	if err := tx.Commit().Error; err != nil {
		return UploadSmallFileResult{}, err
	}

	return UploadSmallFileResult{
		Message:  "upload_success",
		FileID:   newFileMeta.FileID,
		URL:      newFileMeta.StoragePath,
		FileHash: newFileMeta.FileHash,
	}, nil
}

func InitMultipartUploadSession(ctx context.Context, db *gorm.DB, redisClient *redis.Client, userID uint, fileHash string, fileSize int64, fileName string, parentFolderID uint) (InitMultipartResult, error) {
	tx := db.WithContext(ctx).Begin()
	if err := tx.Error; err != nil {
		return InitMultipartResult{}, err
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	existingFile, err := FindActiveFileByHashForUpdate(ctx, tx, fileHash, UploadStatusActive)
	if err == nil {
		if err := IncrementReferenceCount(ctx, tx, existingFile); err != nil {
			tx.Rollback()
			return InitMultipartResult{}, err
		}
		if err := CreateUserFileRelation(ctx, tx, userID, existingFile.FileID, fileName, parentFolderID); err != nil {
			tx.Rollback()
			return InitMultipartResult{}, err
		}
		if err := tx.Commit().Error; err != nil {
			return InitMultipartResult{}, err
		}
		return InitMultipartResult{
			Message:   "quick_pass",
			FileID:    existingFile.FileID,
			URL:       existingFile.StoragePath,
			ChunkSize: 0,
		}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		tx.Rollback()
		return InitMultipartResult{}, err
	}

	s3Client, err := NewS3Client(ctx)
	if err != nil {
		tx.Rollback()
		return InitMultipartResult{}, err
	}

	s3Key := fmt.Sprintf("files/%s/%s", fileHash[:2], fileHash)
	s3Bucket := appconfig.Get().S3.Bucket
	createOut, err := s3Client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(s3Bucket),
		Key:         aws.String(s3Key),
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		tx.Rollback()
		return InitMultipartResult{}, err
	}

	uploadID := ""
	if createOut.UploadId != nil {
		uploadID = *createOut.UploadId
	}

	newFileMeta, err := CreateFileMeta(ctx, tx, fileHash, fileSize, s3Key, UploadStatusUploading, 1)
	if err != nil {
		tx.Rollback()
		return InitMultipartResult{}, err
	}

	redisKey := fmt.Sprintf("upload:%s", uploadID)
	redisValue := fmt.Sprintf("%d:%d:%s:%s:%d", userID, newFileMeta.FileID, s3Key, fileName, parentFolderID)
	if err := redisClient.Set(ctx, redisKey, redisValue, 25*time.Hour).Err(); err != nil {
		tx.Rollback()
		return InitMultipartResult{}, err
	}

	if err := tx.Commit().Error; err != nil {
		return InitMultipartResult{}, err
	}

	return InitMultipartResult{
		Message:   "init_success",
		UploadID:  uploadID,
		FileID:    newFileMeta.FileID,
		S3Key:     s3Key,
		ChunkSize: 5 * 1024 * 1024,
	}, nil
}

func GetPartPresignURL(ctx context.Context, redisClient *redis.Client, userID uint, uploadID string, partNumber int32) (PresignPartResult, error) {
	redisKey := fmt.Sprintf("upload:%s", uploadID)
	redisValue, err := redisClient.Get(ctx, redisKey).Result()
	if err == redis.Nil {
		return PresignPartResult{}, fmt.Errorf("%w: invalid upload ID", ErrInvalid)
	}
	if err != nil {
		return PresignPartResult{}, err
	}

	var storedUserID uint
	var fileMetaID uint
	var s3Key string
	var fileName string
	var parentFolderID int
	_, err = fmt.Sscanf(redisValue, "%d:%d:%s:%s:%d", &storedUserID, &fileMetaID, &s3Key, &fileName, &parentFolderID)
	if err != nil {
		return PresignPartResult{}, err
	}

	if storedUserID != userID {
		return PresignPartResult{}, fmt.Errorf("%w: unauthorized", ErrForbidden)
	}

	s3Client, err := NewS3Client(ctx)
	if err != nil {
		return PresignPartResult{}, err
	}

	presignClient := s3.NewPresignClient(s3Client)
	presignExpires := 15 * time.Minute
	s3Bucket := appconfig.Get().S3.Bucket
	presignResult, err := presignClient.PresignUploadPart(ctx, &s3.UploadPartInput{
		Bucket:     aws.String(s3Bucket),
		Key:        aws.String(s3Key),
		PartNumber: &partNumber,
		UploadId:   aws.String(uploadID),
	}, s3.WithPresignExpires(presignExpires))
	if err != nil {
		return PresignPartResult{}, err
	}

	return PresignPartResult{
		PresignedURL: presignResult.URL,
		ExpiresIn:    presignExpires.Seconds(),
		HTTPMethod:   "PUT",
	}, nil
}

type CompletedPartInput struct {
	PartNumber int32
	ETag       string
}

func CompleteMultipartUploadSession(ctx context.Context, db *gorm.DB, redisClient *redis.Client, userID uint, uploadID string, fileHash string, parts []CompletedPartInput) (CompleteMultipartResult, error) {
	redisKey := fmt.Sprintf("upload:%s", uploadID)
	redisValue, err := redisClient.Get(ctx, redisKey).Result()
	if err == redis.Nil {
		return CompleteMultipartResult{}, fmt.Errorf("%w: upload task not found", ErrInvalid)
	}
	if err != nil {
		return CompleteMultipartResult{}, err
	}

	var storedUserID uint
	var fileMetaID uint
	var s3Key string
	var fileName string
	var parentFolderID uint
	_, err = fmt.Sscanf(redisValue, "%d:%d:%s:%s:%d", &storedUserID, &fileMetaID, &s3Key, &fileName, &parentFolderID)
	if err != nil {
		return CompleteMultipartResult{}, err
	}

	if storedUserID != userID {
		return CompleteMultipartResult{}, fmt.Errorf("%w: unauthorized", ErrForbidden)
	}

	s3Client, err := NewS3Client(ctx)
	if err != nil {
		return CompleteMultipartResult{}, err
	}

	tx := db.WithContext(ctx).Begin()
	if err := tx.Error; err != nil {
		return CompleteMultipartResult{}, err
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	fileMeta, err := GetFileMetaByIDForUpdate(ctx, tx, fileMetaID)
	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CompleteMultipartResult{}, fmt.Errorf("%w: file meta not found", ErrNotFound)
		}
		return CompleteMultipartResult{}, err
	}

	if fileMeta.Status != UploadStatusUploading {
		tx.Rollback()
		return CompleteMultipartResult{}, fmt.Errorf("%w: file is not in uploading state", ErrInvalid)
	}
	if fileMeta.FileHash != fileHash {
		tx.Rollback()
		return CompleteMultipartResult{}, fmt.Errorf("%w: file hash mismatch", ErrInvalid)
	}

	completeParts := make([]types.CompletedPart, len(parts))
	for i, part := range parts {
		completeParts[i] = types.CompletedPart{
			PartNumber: aws.Int32(part.PartNumber),
			ETag:       aws.String(part.ETag),
		}
	}

	s3Bucket := appconfig.Get().S3.Bucket
	_, err = s3Client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s3Bucket),
		Key:      aws.String(s3Key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completeParts,
		},
	})
	if err != nil {
		tx.Rollback()
		return CompleteMultipartResult{}, err
	}

	if err := UpdateFileMetaStatus(ctx, tx, fileMetaID, UploadStatusActive); err != nil {
		tx.Rollback()
		return CompleteMultipartResult{}, err
	}

	if err := CreateUserFileRelation(ctx, tx, userID, fileMetaID, fileName, parentFolderID); err != nil {
		tx.Rollback()
		return CompleteMultipartResult{}, err
	}

	if err := tx.Commit().Error; err != nil {
		return CompleteMultipartResult{}, err
	}

	_ = redisClient.Del(ctx, redisKey).Err()

	return CompleteMultipartResult{
		FileID:   fileMetaID,
		URL:      fileMeta.StoragePath,
		FileHash: fileMeta.FileHash,
	}, nil
}

func RenameFileForUser(ctx context.Context, db *gorm.DB, userID uint, fileID uint, newName string, folderID uint) error {
	return WithTxRetry(ctx, db, 3, func(tx *gorm.DB) error {
		count, err := CountUserFileRenameConflict(ctx, tx, userID, fileID, folderID, newName)
		if err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: rename conflict", ErrConflict)
		}

		if err := UpdateUserFileNameAndFolder(ctx, tx, userID, fileID, newName, folderID); err != nil {
			return err
		}

		return nil
	})
}

func MoveFileForUser(ctx context.Context, db *gorm.DB, userID uint, fileID uint, folderID uint) error {
	return WithTxRetry(ctx, db, 3, func(tx *gorm.DB) error {
		count, err := CountUserFileMoveConflict(ctx, tx, userID, fileID, folderID)
		if err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: move conflict", ErrConflict)
		}

		if err := UpdateUserFileFolder(ctx, tx, userID, fileID, folderID); err != nil {
			return err
		}

		return nil
	})
}

func IncrementShareViewCount(ctx context.Context, db *gorm.DB, shareID uint) (models.FileShare, error) {
	var result models.FileShare
	err := WithTxRetry(ctx, db, 3, func(tx *gorm.DB) error {
		share, err := GetFileShareByIDForUpdate(ctx, tx, shareID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: share not found", ErrNotFound)
			}
			return err
		}

		if share.Status != 1 {
			return fmt.Errorf("%w: share not active", ErrInvalid)
		}
		if !share.ExpireAt.IsZero() && time.Now().After(share.ExpireAt) {
			return fmt.Errorf("%w: share expired", ErrInvalid)
		}
		if share.MaxViews > 0 && share.CurrentViews >= share.MaxViews {
			return fmt.Errorf("%w: share view limit reached", ErrConflict)
		}

		if err := IncrementFileShareViews(ctx, tx, share.ShareID); err != nil {
			return err
		}

		share.CurrentViews += 1
		result = share
		return nil
	})

	return result, err
}

func ListFilesForUser(ctx context.Context, db *gorm.DB, userID uint, folderID string) ([]models.UserFileRelation, error) {
	return ListUserFilesByFolder(ctx, db, userID, folderID)
}
