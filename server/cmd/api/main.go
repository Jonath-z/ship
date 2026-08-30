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
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/accessories"
	"github.com/Jonath-z/ship/server/internal/audit"
	"github.com/Jonath-z/ship/server/internal/auth"
	"github.com/Jonath-z/ship/server/internal/domains"
	"github.com/Jonath-z/ship/server/internal/environments"
	"github.com/Jonath-z/ship/server/internal/environmentvariables"
	"github.com/Jonath-z/ship/server/internal/monitoring"
	"github.com/Jonath-z/ship/server/internal/platform/buildinfo"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	shipcrypto "github.com/Jonath-z/ship/server/internal/platform/crypto"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/internal/platform/health"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
	"github.com/Jonath-z/ship/server/internal/platform/logging"
	shipredis "github.com/Jonath-z/ship/server/internal/platform/redis"
	"github.com/Jonath-z/ship/server/internal/projects"
	shipservices "github.com/Jonath-z/ship/server/internal/services"
	"github.com/Jonath-z/ship/server/internal/setup"
	"github.com/Jonath-z/ship/server/internal/users"
	"github.com/Jonath-z/ship/server/internal/volumes"
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "apply the schema and exit")
	migrateDown := flag.Bool("migrate-down", false, "remove the schema and exit")
	rotateEncryption := flag.Bool("rotate-encryption", false, "rewrap encrypted values with the active master key and exit")
	healthcheckOnly := flag.Bool("healthcheck", false, "check dependencies and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.Summary("ship-api"))
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
			logger.Error("ship-api health check failed", "error", err)
			os.Exit(1)
		}
		return
	}
	if err := run(cfg, logger, *migrateOnly, *migrateDown, *rotateEncryption); err != nil {
		logger.Error("ship-api stopped", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger, migrateOnly, migrateDown, rotateEncryption bool) error {
	selectedCommands := 0
	for _, selected := range []bool{migrateOnly, migrateDown, rotateEncryption} {
		if selected {
			selectedCommands++
		}
	}
	if selectedCommands > 1 {
		return errors.New("-migrate-only, -migrate-down, and -rotate-encryption cannot be used together")
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
	auditService := audit.NewService(db.ORM)
	keyProvider, err := shipcrypto.ProviderFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("configure encryption: %w", err)
	}
	vault := shipcrypto.NewVault(db.ORM, keyProvider, auditService)
	if rotateEncryption {
		result, err := vault.Rotate(ctx)
		if err != nil {
			return err
		}
		logger.Info("encryption rotation complete", "active_key_id", result.ActiveKeyID, "rewrapped", result.Rewrapped)
		return nil
	}
	redisClient, err := shipredis.Open(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer redisClient.Close()
	authService, err := auth.NewService(db.ORM, redisClient, cfg, auditService)
	if err != nil {
		return fmt.Errorf("configure authentication: %w", err)
	}
	userService := users.NewService(db.ORM, authService, auditService)
	projectService := projects.NewService(projects.NewRepository(db.ORM), auditService)
	environmentService := environments.NewService(environments.NewRepository(db.ORM), auditService)
	shipService := shipservices.NewService(shipservices.NewRepository(db.ORM), auditService)
	configurationValueService := environmentvariables.NewService(environmentvariables.NewRepository(db.ORM), vault, auditService)
	accessoryService := accessories.NewService(accessories.NewRepository(db.ORM), auditService, configurationValueService)
	volumeService := volumes.NewService(volumes.NewRepository(db.ORM), auditService)
	domainService := domains.NewService(domains.NewRepository(db.ORM), auditService)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		return fmt.Errorf("configure Gin: %w", err)
	}
	router.Use(httpx.Middleware(logger), httpx.SecurityHeaders(cfg.SecureCookies()))
	routes := httpx.NewRouter(router, authService.Authorize)
	health.RegisterRoutes(routes, "ship-api", map[string]health.Check{
		"postgres": db.Ping,
		"redis": func(ctx context.Context) error {
			return redisClient.Ping(ctx).Err()
		},
	})
	auth.RegisterRoutes(routes, authService)
	setup.RegisterRoutes(routes, cfg, db, authService, auditService)
	monitoring.RegisterRoutes(routes, cfg, db, redisClient)
	users.RegisterRoutes(routes, cfg, userService)
	projects.RegisterRoutes(routes, cfg, projectService)
	environments.RegisterRoutes(routes, cfg, environmentService)
	shipservices.RegisterRoutes(routes, cfg, shipService)
	accessories.RegisterRoutes(routes, cfg, accessoryService)
	volumes.RegisterRoutes(routes, cfg, volumeService)
	domains.RegisterRoutes(routes, cfg, domainService)
	environmentvariables.RegisterRoutes(routes, cfg, configurationValueService)
	audit.RegisterRoutes(routes, auditService)
	httpx.RegisterOpenAPIRoute(routes)
	routes.NoRoute(httpx.NotFound)
	if !cfg.SecureCookies() {
		logger.Warn("Ship is using insecure HTTP bootstrap mode; configure an HTTPS public URL before production use", "public_url", cfg.PublicURL)
	}

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
