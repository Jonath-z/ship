package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jonath-z/ship/server/internal/platform/config"
)

func TestCollectReportsComponentsAndHidesSensitiveConfiguration(t *testing.T) {
	secretDatabasePath := "/data/ship/database-secret/ship.db"
	secretRedisURL := "redis://:redis-secret@localhost/0"
	collector := New(config.Config{
		Environment:  "test",
		Hostname:     "ship.example.com",
		APIAddr:      ":8080",
		WorkerAddr:   ":8081",
		DataDir:      "/data/ship",
		DatabasePath: secretDatabasePath,
		RedisURL:     secretRedisURL,
	}, Dependencies{
		Postgres: func(context.Context) error { return nil },
		Redis:    func(context.Context) error { return nil },
		WorkerLastSeen: func(context.Context) (time.Time, error) {
			return time.Now(), nil
		},
	})

	snapshot := collector.Collect(context.Background())
	if snapshot.Status != "ok" {
		t.Fatalf("expected an operational system, got %q", snapshot.Status)
	}
	if snapshot.Components["worker"].Status != "ok" {
		t.Fatalf("expected a healthy worker, got %#v", snapshot.Components["worker"])
	}

	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{secretDatabasePath, secretRedisURL, "database-secret", "redis-secret"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("system status exposed sensitive configuration %q", secret)
		}
	}
}

func TestCollectDegradesWhenWorkerHeartbeatIsMissing(t *testing.T) {
	collector := New(config.Config{}, Dependencies{
		Postgres: func(context.Context) error { return nil },
		Redis:    func(context.Context) error { return nil },
		WorkerLastSeen: func(context.Context) (time.Time, error) {
			return time.Time{}, errors.New("missing")
		},
	})

	snapshot := collector.Collect(context.Background())
	if snapshot.Status != "degraded" {
		t.Fatalf("expected degraded status, got %q", snapshot.Status)
	}
	if snapshot.Components["worker"].Status != "unknown" {
		t.Fatalf("expected unknown worker status, got %#v", snapshot.Components["worker"])
	}
}
