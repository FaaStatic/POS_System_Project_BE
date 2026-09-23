package domain

import (
	"context"
	"time"
)

type Session struct {
	UserID       string    `json:"user_id"`
	Role         string    `json:"role"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type SessionRepository interface {
	CreateSession(ctx context.Context, session *Session, key string, ttl time.Duration) error
	DeleteSession(ctx context.Context, key string) error
	PopSessionByRefreshToken(ctx context.Context, key string) (*Session, error)
	Exists(ctx context.Context, key string) (bool, error)
}
