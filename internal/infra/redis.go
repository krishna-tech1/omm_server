package infra

import (
	"context"

	"github.com/redis/go-redis/v9"

	"one-more-mile/server/internal/config"
)

func NewRedisClient(ctx context.Context, cfg config.Config) (*redis.Client, error) {
	opt, _ := redis.ParseURL(cfg.RedisUrl)
	client := redis.NewClient(opt)

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}
