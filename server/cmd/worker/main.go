// Command worker runs Ship's asynchronous worker process.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	redisclient "github.com/redis/go-redis/v9"

	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/internal/platform/health"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
	"github.com/Jonath-z/ship/server/internal/platform/logging"
	shipredis "github.com/Jonath-z/ship/server/internal/platform/redis"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(1)
	}
	logger := logging.New(cfg.LogLevel)
	if err := run(cfg, logger); err != nil {
		logger.Error("ship-worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	redisClient, err := shipredis.Open(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer redisClient.Close()
	if err := shipredis.SetWorkerHeartbeat(ctx, redisClient, time.Now()); err != nil {
		return fmt.Errorf("publish worker heartbeat: %w", err)
	}
	go publishHeartbeats(ctx, logger, redisClient)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		return fmt.Errorf("configure Gin: %w", err)
	}
	router.Use(httpx.Middleware(logger))
	router.GET("/healthz", health.Handler("ship-worker", map[string]health.Check{
		"postgres": db.Ping,
		"redis": func(ctx context.Context) error {
			return redisClient.Ping(ctx).Err()
		},
	}))
	router.NoRoute(httpx.NotFound)

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("ship-worker health server listening", "addr", cfg.WorkerAddr)
		serverErrors <- router.Run(cfg.WorkerAddr)
	}()

	select {
	case <-ctx.Done():
		logger.Info("ship-worker shutting down")
		return nil
	case err := <-serverErrors:
		return fmt.Errorf("serve worker health endpoint: %w", err)
	}
}

func publishHeartbeats(ctx context.Context, logger *slog.Logger, client *redisclient.Client) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case at := <-ticker.C:
			if err := shipredis.SetWorkerHeartbeat(ctx, client, at); err != nil && ctx.Err() == nil {
				logger.Warn("worker heartbeat failed", "error", err)
			}
		}
	}
}
