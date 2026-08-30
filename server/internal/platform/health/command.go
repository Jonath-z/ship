package health

import (
	"context"
	"fmt"

	"github.com/Jonath-z/ship/server/internal/platform/database"
	shipredis "github.com/Jonath-z/ship/server/internal/platform/redis"
)

// CheckDependencies is used by the container-native health commands. It does
// not start an HTTP server or run migrations.
func CheckDependencies(ctx context.Context, databaseURL, redisURL string) error {
	db, err := database.Open(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("postgres health check: %w", err)
	}
	defer db.Close()

	redisClient, err := shipredis.Open(ctx, redisURL)
	if err != nil {
		return fmt.Errorf("redis health check: %w", err)
	}
	defer redisClient.Close()

	return nil
}
