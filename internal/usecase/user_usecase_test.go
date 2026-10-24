package usecase

import (
	"context"
	"testing"

	"shop_project_be/internal/domain"
	"shop_project_be/pkg/jwt"

	"go.uber.org/zap"
)

type fakeSessionRepo struct {
	domain.SessionRepository

	sessions map[string]*domain.Session
}

func (f *fakeSessionRepo) PopSession(ctx context.Context, key string) (*domain.Session, error) {
	session := f.sessions[key]
	delete(f.sessions, key)
	return session, nil
}

func (f *fakeSessionRepo) DeleteSession(ctx context.Context, key string) error {
	delete(f.sessions, key)
	return nil
}

func TestLogout_RevokesRefreshWithoutBody(t *testing.T) {
	accessHash := jwt.HashToken("access-token")
	refreshHash := jwt.HashToken("refresh-token")
	session := &domain.Session{AccessToken: accessHash, RefreshToken: refreshHash}
	repo := &fakeSessionRepo{sessions: map[string]*domain.Session{
		"session:" + accessHash:  session,
		"refresh:" + refreshHash: session,
	}}
	uc := &userUsecase{sessionRepo: repo, log: zap.NewNop()}

	if err := uc.Logout(context.Background(), "access-token", nil); err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
	if len(repo.sessions) != 0 {
		t.Errorf("sessions left after logout: %v, want none", repo.sessions)
	}
}
