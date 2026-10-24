package cache

import (
	"context"
	"fmt"
	envconfig "shop_project_be/config/env_config"

	"github.com/gofiber/fiber/v3"
	redisStorage "github.com/gofiber/storage/redis/v3"
	"github.com/redis/go-redis/v9"
)

func InitRedis(cfg *envconfig.RedisConfig) (*redis.Client, error) {

	rdb := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.Db,
		PoolSize:     10,
		MinIdleConns: 5,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return rdb, nil

}

// NewLimiterStorage memakai client yang sama dengan sesi, bukan pool kedua.
func NewLimiterStorage(rdb *redis.Client) fiber.Storage {
	return redisStorage.NewFromConnection(rdb)
}
