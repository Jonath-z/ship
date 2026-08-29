// Command api runs the Ship control-plane API with Gin.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/internal/platform/health"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
	"github.com/Jonath-z/ship/server/internal/platform/logging"
	shipredis "github.com/Jonath-z/ship/server/internal/platform/redis"
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "apply the schema and exit")
	migrateDown := flag.Bool("migrate-down", false, "remove the schema and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(1)
	}
	logger := logging.New(cfg.LogLevel)
	if err := run(cfg, logger, *migrateOnly, *migrateDown); err != nil {
		logger.Error("ship-api stopped", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger, migrateOnly, migrateDown bool) error {
	if migrateOnly && migrateDown {
		return errors.New("-migrate-only and -migrate-down cannot be used together")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if migrateDown {
		logger.Info("removing database schema")
		return database.MigrateDown(ctx, cfg.DatabaseURL)
	}
	if migrateOnly || cfg.RunMigrations {
		logger.Info("applying database schema")
		if err := database.MigrateUp(ctx, cfg.DatabaseURL); err != nil {
			return err
		}
		if migrateOnly {
			return nil
		}
	}

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

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		return fmt.Errorf("configure Gin: %w", err)
	}
	router.Use(httpx.Middleware(logger))
	router.GET("/healthz", health.Handler("ship-api", map[string]health.Check{
		"postgres": db.Ping,
		"redis": func(ctx context.Context) error {
			return redisClient.Ping(ctx).Err()
		},
	}))
	router.GET("/openapi.yaml", func(c *gin.Context) {
		c.Data(200, "application/yaml", httpx.OpenAPISpec)
	})
	router.NoRoute(httpx.NotFound)

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("ship-api listening", "addr", cfg.APIAddr)
		serverErrors <- router.Run(cfg.APIAddr)
	}()

	select {
	case <-ctx.Done():
		logger.Info("ship-api shutting down")
		return nil
	case err := <-serverErrors:
		return fmt.Errorf("serve API: %w", err)
	}
}
