package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Platform string

const (
	PlatformAndroid Platform = "android"
	PlatformIos     Platform = "ios"
)

type DeviceToken struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Token      string     `gorm:"uniqueIndex;not null" json:"token"`
	UserID     *uuid.UUID `gorm:"type:uuid;index" json:"user_id,omitempty"`
	DeviceID   string     `gorm:"not null" json:"device_id"`
	Platform   Platform   `gorm:"type:varchar(16);not null" json:"platform"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastUsedAt time.Time  `json:"last_used_at"`
}

func (dt *DeviceToken) BeforeCreate(tx *gorm.DB) (err error) {
	if dt.ID == uuid.Nil {
		dt.ID = uuid.New()
	}
	return nil
}

type Payload struct {
	Title string
	Body  string
	Data  map[string]string
}

type DeviceTokenRepository interface {
	RegisterDeviceToken(ctx context.Context, dt *DeviceToken) error
	DeleteDeviceToken(ctx context.Context, tokens []string) error
	DetachDeviceTokenFromUser(ctx context.Context, tokens string) error
	GetDeviceTokensByUserID(ctx context.Context, userID uuid.UUID) ([]string, error)
}

type DeviceTokenUsecase interface {
	RegisterDevice(ctx context.Context, userID, token, platform, deviceID string) error
	HandleLogout(ctx context.Context, token string) error
	NotifyPaymentResult(ctx context.Context, userID, orderID string, success bool, amount int64) error
}
