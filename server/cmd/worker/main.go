// Command worker runs Ship's asynchronous worker process.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	redisclient "github.com/redis/go-redis/v9"

	"github.com/Jonath-z/ship/server/internal/audit"
	"github.com/Jonath-z/ship/server/internal/configuration"
	"github.com/Jonath-z/ship/server/internal/deployments"
	"github.com/Jonath-z/ship/server/internal/kamal"
	"github.com/Jonath-z/ship/server/internal/platform/buildinfo"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	shipcrypto "github.com/Jonath-z/ship/server/internal/platform/crypto"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/internal/platform/health"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
	"github.com/Jonath-z/ship/server/internal/platform/jobs"
	"github.com/Jonath-z/ship/server/internal/platform/logging"
	shipredis "github.com/Jonath-z/ship/server/internal/platform/redis"
	"github.com/Jonath-z/ship/server/internal/sshkeys"
)

func main() {
	healthcheckOnly := flag.Bool("healthcheck", false, "check dependencies and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.Summary("ship-worker"))
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(1)
	}
	logger := logging.New(cfg.LogLevel)
	if *healthcheckOnly {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := health.CheckDependencies(ctx, cfg.DatabaseURL, cfg.RedisURL); err != nil {
			logger.Error("ship-worker health check failed", "error", err)
			os.Exit(1)
		}
		return
	}
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

	// SH-062: assert the pinned Kamal runtime at startup. Development hosts
	// without the binary run everything except real deployments.
	engine := kamal.NewCLIEngine()
	if version, err := engine.Version(ctx); err != nil {
		if cfg.Environment == "production" {
			return fmt.Errorf("kamal runtime check: %w", err)
		}
		logger.Warn("kamal is not installed; deployments will fail until it is", "error", err)
	} else {
		logger.Info("kamal runtime ready", "version", version)
	}

	keyProvider, err := shipcrypto.ProviderFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("configure encryption: %w", err)
	}
	auditService := audit.NewService(db.ORM)
	vault := shipcrypto.NewVault(db.ORM, keyProvider, auditService)
	queue := jobs.NewQueue(redisClient, logger)
	runner := &deployments.Runner{
		DB: db.ORM, Redis: redisClient, Queue: queue,
		Configuration: configuration.NewRepository(db.ORM),
		Vault:         vault,
		SSHKeys:       sshkeys.NewService(db.ORM, vault, auditService),
		Engine:        engine,
		DataDir:       cfg.DataDir,
	}

	// A worker restart mid-deployment marks the run failed, never stuck.
	stale, err := queue.RecoverStale(ctx)
	if err != nil {
		return fmt.Errorf("recover stale jobs: %w", err)
	}
	for _, job := range stale {
		if job.Type == deployments.JobTypeDeploy {
			runner.MarkStale(ctx, job, nil)
		}
	}
	go queue.Consume(ctx, map[string]jobs.Handler{
		deployments.JobTypeDeploy: runner.Handle,
	}, func(ctx context.Context, job jobs.Job, err error) {
		if job.Type == deployments.JobTypeDeploy {
			runner.MarkStale(ctx, job, err)
		}
	})

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
