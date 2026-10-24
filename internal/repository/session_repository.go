package repository

import (
	"context"
	"errors"
	"shop_project_be/internal/domain"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

type sessionRepository struct {
	rdb *redis.Client
}

func NewSessionRepository(rdb *redis.Client) domain.SessionRepository {
	return &sessionRepository{rdb: rdb}
}

func (s *sessionRepository) CreateSession(ctx context.Context, session *domain.Session, key string, ttl time.Duration) error {
	data, err := sonic.Marshal(session)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, key, data, ttl).Err()
}

func (s *sessionRepository) DeleteSession(ctx context.Context, key string) error {
	return s.rdb.Del(ctx, key).Err()
}

func (s *sessionRepository) Exists(ctx context.Context, key string) (bool, error) {
	exists, err := s.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// RevokeUserSessions memindai semua sesi dan menghapus milik userID. Tidak
// butuh indeks per user, jadi sesi yang dibuat sebelum fitur ini juga ikut tercabut.
// ponytail: O(jumlah sesi aktif) per panggilan; hanya dipakai aksi admin yang
// jarang (nonaktifkan staff, ganti password). Tambah indeks per user kalau sesi
// aktif mencapai puluhan ribu.
func (s *sessionRepository) RevokeUserSessions(ctx context.Context, userID string) error {
	for _, pattern := range []string{"session:*", "refresh:*"} {
		var cursor uint64
		for {
			keys, next, err := s.rdb.Scan(ctx, cursor, pattern, 500).Result()
			if err != nil {
				return err
			}
			if err := s.deleteOwnedBy(ctx, keys, userID); err != nil {
				return err
			}
			if cursor = next; cursor == 0 {
				break
			}
		}
	}
	return nil
}

// deleteOwnedBy membaca satu halaman SCAN dengan satu MGET lalu menghapus key
// milik userID dalam satu DEL.
func (s *sessionRepository) deleteOwnedBy(ctx context.Context, keys []string, userID string) error {
	if len(keys) == 0 {
		return nil
	}
	values, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return err
	}
	owned := make([]string, 0, len(keys))
	for i, v := range values {
		raw, ok := v.(string) // nil: key sudah expired di antara SCAN dan MGET
		if !ok {
			continue
		}
		var session domain.Session
		if sonic.UnmarshalString(raw, &session) == nil && session.UserID == userID {
			owned = append(owned, keys[i])
		}
	}
	if len(owned) == 0 {
		return nil
	}
	return s.rdb.Del(ctx, owned...).Err()
}

func (s *sessionRepository) PopSession(ctx context.Context, key string) (*domain.Session, error) {
	data, err := s.rdb.GetDel(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	var session domain.Session
	if err := sonic.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}
