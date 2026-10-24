package usecase

import (
	"errors"
	"fmt"

	"shop_project_be/internal/domain"

	"go.uber.org/zap"
)

// passOrInternal meneruskan err apa adanya bila termasuk salah satu kinds (error
// bisnis yang aman dilihat klien); selain itu dicatat dan disembunyikan di balik
// ErrInternal supaya detail DB tidak bocor ke response.
func passOrInternal(log *zap.Logger, err error, msg string, kinds ...error) error {
	for _, kind := range kinds {
		if errors.Is(err, kind) {
			return err
		}
	}
	log.Error(msg, zap.Error(err))
	return fmt.Errorf("%s: %w", msg, domain.ErrInternal)
}
