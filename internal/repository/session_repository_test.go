package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"shop_project_be/internal/domain"
	"shop_project_be/internal/repository"

	"github.com/redis/go-redis/v9"
)

func TestRevokeUserSessions_OnlyRemovesThatUser(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set TEST_REDIS_ADDR to run this test against a real Redis")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	repo := repository.NewSessionRepository(rdb)
	ctx := context.Background()

	keys := map[string]string{
		"session:a1": "victim", "refresh:a2": "victim",
		"session:b1": "other", "refresh:b2": "other",
	}
	for key, user := range keys {
		if err := repo.CreateSession(ctx, &domain.Session{UserID: user}, key, time.Minute); err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
	}
	t.Cleanup(func() {
		for key := range keys {
			rdb.Del(ctx, key)
		}
	})

	if err := repo.RevokeUserSessions(ctx, "victim"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	for key, user := range keys {
		alive, _ := repo.Exists(ctx, key)
		if alive != (user == "other") {
			t.Errorf("%s (user %s) alive=%v", key, user, alive)
		}
	}
}
