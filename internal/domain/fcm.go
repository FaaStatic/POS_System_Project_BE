package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Platform string

type DeviceToken struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Token      string     `json:"token"`
	UserID     *uuid.UUID `gorm:"type:uuid" json:"user_id,omitempty"`
	DeviceID   string     `json:"device_id"`
	Platform   Platform   `gorm:"type:varchar(16)" json:"platform"`
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
	DeleteDeviceTokensByUser(ctx context.Context, userID uuid.UUID) error
	ListDevices(ctx context.Context, platform string) ([]DeviceListItem, error)
	DeleteDeviceByID(ctx context.Context, id uuid.UUID) error
}

// DeviceListItem: Username kosong berarti device sudah logout (token tidak
// terpasang ke user mana pun).
type DeviceListItem struct {
	ID         uuid.UUID  `gorm:"column:id" json:"id"`
	DeviceID   string     `gorm:"column:device_id" json:"device_id"`
	Platform   string     `gorm:"column:platform" json:"platform"`
	UserID     *uuid.UUID `gorm:"column:user_id" json:"user_id,omitempty"`
	Username   *string    `gorm:"column:username" json:"username,omitempty"`
	LastUsedAt time.Time  `gorm:"column:last_used_at" json:"last_used_at"`
	CreatedAt  time.Time  `gorm:"column:created_at" json:"created_at"`
}

type DeviceTokenUsecase interface {
	RegisterDevice(ctx context.Context, userID, token, platform, deviceID string) error
	HandleLogout(ctx context.Context, token string) error
	NotifyPaymentResult(ctx context.Context, userID, orderID string, success bool, amount int64) error
	ListDevices(ctx context.Context, platform string) ([]DeviceListItem, error)
	RevokeDevice(ctx context.Context, id string) error
}
