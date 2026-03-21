package cmd

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yurin-kami/CloudKaho/models"
	"gorm.io/gorm"
)

func StartCleanupJob(fileConnection *gorm.DB, redisClient *redis.Client) {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		for range ticker.C {
			cleanupStaleUploads(fileConnection, redisClient)
		}
	}()
}

func cleanupStaleUploads(fileConnection *gorm.DB, redisClient *redis.Client) {
	ctx := context.Background()

	var staleFiles []models.FileMeta
	thresholdTime := time.Now().Add(-26 * time.Hour)

	if err := fileConnection.Where("status = ? AND create_at < ?", "2", thresholdTime).Limit(100).Find(&staleFiles).Error; err != nil {
		log.Printf("Error fetching stale uploads: %v", err)
		return
	}

	if len(staleFiles) == 0 {
		return
	}

	log.Printf("Found %d stale uploads, cleaning up...", len(staleFiles))

	for _, fileMeta := range staleFiles {
		fileConnection.Model(&fileMeta).WithContext(ctx).Update("status", 3)
	}
}
