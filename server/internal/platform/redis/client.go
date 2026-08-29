// Package redis owns Redis connectivity for transient jobs, locks, and events.
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

const (
	workerHeartbeatKey = "ship:worker:heartbeat"
	workerHeartbeatTTL = 20 * time.Second
)

var ErrWorkerHeartbeatMissing = errors.New("worker heartbeat is missing")

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

func SetWorkerHeartbeat(ctx context.Context, client *redisclient.Client, at time.Time) error {
	return client.Set(ctx, workerHeartbeatKey, at.UTC().UnixMilli(), workerHeartbeatTTL).Err()
}

func WorkerLastSeen(ctx context.Context, client *redisclient.Client) (time.Time, error) {
	milliseconds, err := client.Get(ctx, workerHeartbeatKey).Int64()
	if errors.Is(err, redisclient.Nil) {
		return time.Time{}, ErrWorkerHeartbeatMissing
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("read worker heartbeat: %w", err)
	}
	return time.UnixMilli(milliseconds).UTC(), nil
}
