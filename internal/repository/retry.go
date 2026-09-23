package repository

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const maxTxRetries = 3

func isRetryableTxError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01" || pgErr.Code == "23505")
}

func runTxDB(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	var err error
	for attempt := 0; attempt < maxTxRetries; attempt++ {
		err = db.WithContext(ctx).Transaction(fn)
		if err == nil || !isRetryableTxError(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(txBackoff(attempt)):
		}
	}
	return err
}

func txBackoff(attempt int) time.Duration {
	base := time.Duration(5<<attempt) * time.Millisecond
	return base + time.Duration(rand.Int63n(int64(base)+1))
}
