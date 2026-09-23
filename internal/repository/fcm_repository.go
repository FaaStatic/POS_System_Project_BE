package repository

import (
	"context"
	"shop_project_be/internal/domain"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type deviceTokenRepository struct {
	db *gorm.DB
}

func NewDeviceTokenRepository(db *gorm.DB) domain.DeviceTokenRepository {
	return &deviceTokenRepository{db: db}
}

func (d *deviceTokenRepository) DeleteDeviceToken(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	return d.db.WithContext(ctx).Where("token IN ?", tokens).Delete(&domain.DeviceToken{}).Error
}

func (d *deviceTokenRepository) DetachDeviceTokenFromUser(ctx context.Context, tokens string) error {
	return d.db.WithContext(ctx).Model(&domain.DeviceToken{}).Where("token = ?", tokens).Updates(map[string]any{
		"user_id":    nil,
		"updated_at": time.Now(),
	}).Error
}

func (d *deviceTokenRepository) GetDeviceTokensByUserID(ctx context.Context, userID uuid.UUID) ([]string, error) {
	var tokens []string
	err := d.db.WithContext(ctx).Model(&domain.DeviceToken{}).Where("user_id = ?", userID).Pluck("token", &tokens).Error
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

func (d *deviceTokenRepository) RegisterDeviceToken(ctx context.Context, dt *domain.DeviceToken) error {
	dt.LastUsedAt = time.Now()
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "token"}},
		DoUpdates: clause.AssignmentColumns([]string{"user_id", "device_id", "platform", "last_used_at"}),
	}).Create(dt).Error
}
