// Package redis owns Redis connectivity for transient jobs, locks, and events.
package redis

import (
	"context"
	"fmt"

	redisclient "github.com/redis/go-redis/v9"
)

func Open(ctx context.Context, redisURL string) (*redisclient.Client, error) {
	options, err := redisclient.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}
	client := redisclient.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect to redis: %w", err)
	}
	return client, nil
}
