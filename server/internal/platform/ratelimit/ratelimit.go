// Package ratelimit implements atomic, Redis-backed fixed-window limits.
package ratelimit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

var incrementScript = redisclient.NewScript(`
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('PTTL', KEYS[1])
return {current, ttl}
`)

type Decision struct {
	Count      int64
	Limit      int64
	RetryAfter time.Duration
}

func (decision Decision) Blocked() bool {
	return decision.Count >= decision.Limit
}

type Limiter struct {
	client *redisclient.Client
	secret []byte
}

func New(client *redisclient.Client, secret string) *Limiter {
	return &Limiter{client: client, secret: []byte(secret)}
}

func (limiter *Limiter) Key(policy, value string) string {
	mac := hmac.New(sha256.New, limiter.secret)
	_, _ = mac.Write([]byte(value))
	return "ship:ratelimit:" + policy + ":" + hex.EncodeToString(mac.Sum(nil))
}

func (limiter *Limiter) Current(ctx context.Context, key string, limit int64) (Decision, error) {
	result, err := limiter.client.Pipelined(ctx, func(pipe redisclient.Pipeliner) error {
		pipe.Get(ctx, key)
		pipe.PTTL(ctx, key)
		return nil
	})
	if err != nil && err != redisclient.Nil {
		return Decision{}, fmt.Errorf("read rate limit: %w", err)
	}
	decision := Decision{Limit: limit}
	if len(result) != 2 {
		return decision, nil
	}
	if value, parseErr := strconv.ParseInt(result[0].(*redisclient.StringCmd).Val(), 10, 64); parseErr == nil {
		decision.Count = value
	}
	decision.RetryAfter = result[1].(*redisclient.DurationCmd).Val()
	return decision, nil
}

func (limiter *Limiter) Hit(ctx context.Context, key string, limit int64, window time.Duration) (Decision, error) {
	value, err := incrementScript.Run(ctx, limiter.client, []string{key}, window.Milliseconds()).Slice()
	if err != nil {
		return Decision{}, fmt.Errorf("increment rate limit: %w", err)
	}
	if len(value) != 2 {
		return Decision{}, errorsNewUnexpectedResult()
	}
	count, ok := value[0].(int64)
	if !ok {
		return Decision{}, errorsNewUnexpectedResult()
	}
	ttlMilliseconds, ok := value[1].(int64)
	if !ok {
		return Decision{}, errorsNewUnexpectedResult()
	}
	return Decision{Count: count, Limit: limit, RetryAfter: time.Duration(ttlMilliseconds) * time.Millisecond}, nil
}

func (limiter *Limiter) Reset(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return limiter.client.Del(ctx, keys...).Err()
}

func errorsNewUnexpectedResult() error {
	return fmt.Errorf("unexpected Redis rate-limit response")
}
