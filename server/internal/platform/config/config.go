// Package config loads and validates Ship's process configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultDatabaseURL = "postgres://ship:ship@localhost:5432/ship?sslmode=disable"
	defaultRedisURL    = "redis://localhost:6379/0"
)

type Config struct {
	Environment   string
	Hostname      string
	APIAddr       string
	WorkerAddr    string
	DataDir       string
	DatabaseURL   string
	RedisURL      string
	RunMigrations bool
	LogLevel      string
}

func Load() (Config, error) {
	runMigrations, err := envBool("SHIP_RUN_MIGRATIONS", false)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Environment:   env("SHIP_ENV", "development"),
		Hostname:      env("SHIP_HOSTNAME", "localhost"),
		APIAddr:       env("SHIP_API_ADDR", ":8080"),
		WorkerAddr:    env("SHIP_WORKER_ADDR", ":8081"),
		DataDir:       env("SHIP_DATA_DIR", "./data/ship"),
		DatabaseURL:   env("DATABASE_URL", defaultDatabaseURL),
		RedisURL:      env("REDIS_URL", defaultRedisURL),
		RunMigrations: runMigrations,
		LogLevel:      strings.ToLower(env("SHIP_LOG_LEVEL", "info")),
	}

	if cfg.APIAddr == cfg.WorkerAddr {
		return Config{}, errors.New("SHIP_API_ADDR and SHIP_WORKER_ADDR must differ")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.RedisURL == "" {
		return Config{}, errors.New("REDIS_URL is required")
	}

	return cfg, nil
}

func env(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) (bool, error) {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", name, err)
	}
	return parsed, nil
}
