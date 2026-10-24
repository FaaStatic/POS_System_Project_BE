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
	return d.db.WithContext(ctx).Model(&domain.DeviceToken{}).Where("token = ?", tokens).Update("user_id", nil).Error
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
		DoUpdates: clause.AssignmentColumns([]string{"user_id", "device_id", "platform", "last_used_at", "updated_at"}),
	}).Create(dt).Error
}

func (d *deviceTokenRepository) DeleteDeviceTokensByUser(ctx context.Context, userID uuid.UUID) error {
	return d.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&domain.DeviceToken{}).Error
}

// ListDevices tanpa paginasi: satu toko hanya punya sedikit device.
// ponytail: tambahkan keyset paging kalau jumlah device mencapai ratusan.
func (d *deviceTokenRepository) ListDevices(ctx context.Context, platform string) ([]domain.DeviceListItem, error) {
	query := d.db.WithContext(ctx).Table("device_tokens").
		Select("device_tokens.id, device_tokens.device_id, device_tokens.platform, device_tokens.user_id, users.username, device_tokens.last_used_at, device_tokens.created_at").
		Joins("LEFT JOIN users ON users.id = device_tokens.user_id")
	if platform != "" {
		query = query.Where("device_tokens.platform = ?", platform)
	}
	var rows []domain.DeviceListItem
	if err := query.Order("device_tokens.last_used_at DESC NULLS LAST").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (d *deviceTokenRepository) DeleteDeviceByID(ctx context.Context, id uuid.UUID) error {
	result := d.db.WithContext(ctx).Where("id = ?", id).Delete(&domain.DeviceToken{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.NotFound("device not found")
	}
	return nil
}
