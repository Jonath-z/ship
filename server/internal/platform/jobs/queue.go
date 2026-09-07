package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

const (
	queueKey  = "ship:jobs:queued"
	activeKey = "ship:jobs:active"
	lockTTL   = 30 * time.Minute
	maxTries  = 3
)

// Job is one unit of asynchronous work. Payload identifies the target record;
// handlers reload state from PostgreSQL, so delivery is at-least-once safe.
type Job struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Tries   int    `json:"tries"`
}

type Handler func(ctx context.Context, job Job) error

type Queue struct {
	client *redisclient.Client
	logger *slog.Logger
}

func NewQueue(client *redisclient.Client, logger *slog.Logger) *Queue {
	return &Queue{client: client, logger: logger}
}

func (queue *Queue) Enqueue(ctx context.Context, job Job) error {
	encoded, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encode job: %w", err)
	}
	return queue.client.LPush(ctx, queueKey, encoded).Err()
}

// RecoverStale returns jobs left in the active list by a worker that died
// mid-run. The caller marks their records failed — a crashed deployment must
// never stay stuck in a non-terminal state.
func (queue *Queue) RecoverStale(ctx context.Context) ([]Job, error) {
	raw, err := queue.client.LRange(ctx, activeKey, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	if err := queue.client.Del(ctx, activeKey).Err(); err != nil {
		return nil, err
	}
	stale := make([]Job, 0, len(raw))
	for _, entry := range raw {
		var job Job
		if json.Unmarshal([]byte(entry), &job) == nil {
			stale = append(stale, job)
		}
	}
	return stale, nil
}

// Consume processes jobs until the context ends. A failing job is retried
// with backoff up to maxTries; exhausted jobs are handed to failed, which
// marks the underlying record visibly — that is Ship's dead-letter.
func (queue *Queue) Consume(ctx context.Context, handlers map[string]Handler, failed func(ctx context.Context, job Job, err error)) {
	for {
		if ctx.Err() != nil {
			return
		}
		entry, err := queue.client.BLMove(ctx, queueKey, activeKey, "RIGHT", "LEFT", 5*time.Second).Result()
		if errors.Is(err, redisclient.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			queue.logger.Warn("job queue read failed", "error", err)
			time.Sleep(time.Second)
			continue
		}

		var job Job
		if unmarshalErr := json.Unmarshal([]byte(entry), &job); unmarshalErr != nil {
			queue.logger.Error("job payload is not valid JSON; dropping", "entry", entry)
			queue.client.LRem(ctx, activeKey, 1, entry)
			continue
		}
		handler, known := handlers[job.Type]
		if !known {
			queue.logger.Error("no handler for job type; dropping", "type", job.Type)
			queue.client.LRem(ctx, activeKey, 1, entry)
			continue
		}

		handlerErr := handler(ctx, job)
		queue.client.LRem(ctx, activeKey, 1, entry)
		if handlerErr == nil {
			continue
		}
		job.Tries++
		if job.Tries >= maxTries {
			queue.logger.Error("job failed permanently", "type", job.Type, "payload", job.Payload, "error", handlerErr)
			if failed != nil {
				failed(ctx, job, handlerErr)
			}
			continue
		}
		queue.logger.Warn("job failed; requeueing", "type", job.Type, "try", job.Tries, "error", handlerErr)
		time.Sleep(time.Duration(job.Tries) * 2 * time.Second)
		if err := queue.Enqueue(ctx, job); err != nil {
			queue.logger.Error("requeue failed", "error", err)
		}
	}
}

// AcquireEnvironmentLock serializes deployments per environment. It returns a
// release function, or false when another deployment holds the lock.
func (queue *Queue) AcquireEnvironmentLock(ctx context.Context, environmentID, token string) (func(), bool, error) {
	key := "ship:lock:environment:" + environmentID
	acquired, err := queue.client.SetNX(ctx, key, token, lockTTL).Result()
	if err != nil || !acquired {
		return nil, false, err
	}
	release := func() {
		// Only the holder may release: guard against expiry + reacquisition.
		script := redisclient.NewScript(`if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`)
		_ = script.Run(context.Background(), queue.client, []string{key}, token).Err()
	}
	return release, true, nil
}
