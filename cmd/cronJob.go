package cmd

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yurin-kami/CloudKaho/service"
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

	thresholdTime := time.Now().Add(-26 * time.Hour)

	staleFiles, err := service.FindStaleUploads(ctx, fileConnection, 2, thresholdTime, 100)
	if err != nil {
		log.Printf("Error fetching stale uploads: %v", err)
		return
	}

	if len(staleFiles) == 0 {
		return
	}

	log.Printf("Found %d stale uploads, cleaning up...", len(staleFiles))

	if err := service.WithTxRetry(ctx, fileConnection, 3, func(tx *gorm.DB) error {
		for _, fileMeta := range staleFiles {
			if err := service.UpdateFileMetaStatus(ctx, tx, fileMeta.FileID, 3); err != nil {
				log.Printf("Error updating stale upload status: %v", err)
				return err
			}
		}
		return nil
	}); err != nil {
		log.Printf("Error committing stale upload updates: %v", err)
	}

}


