package service

import (
	"context"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

const (
	defaultMaxTxRetries = 3
	baseRetryDelay      = 50 * time.Millisecond
)

type TxFunc func(tx *gorm.DB) error

func WithTxRetry(ctx context.Context, db *gorm.DB, maxRetries int, fn TxFunc) error {
	if maxRetries <= 0 {
		maxRetries = defaultMaxTxRetries
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		tx := db.WithContext(ctx).Begin()
		if err := tx.Error; err != nil {
			return err
		}

		err := fn(tx)
		if err != nil {
			_ = tx.Rollback()
			if isRetryableTxError(err) && attempt < maxRetries {
				sleepForRetry(attempt)
				continue
			}
			return err
		}

		if err := tx.Commit().Error; err != nil {
			if isRetryableTxError(err) && attempt < maxRetries {
				sleepForRetry(attempt)
				continue
			}
			return err
		}

		return nil
	}

	return nil
}

func isRetryableTxError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1205, 1213: // lock wait timeout, deadlock
			return true
		}
	}
	return false
}

func sleepForRetry(attempt int) {
	delay := baseRetryDelay * time.Duration(attempt+1)
	time.Sleep(delay)
}
