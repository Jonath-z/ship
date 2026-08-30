// Package config loads and validates Ship's process configuration.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDatabaseURL = "postgres://ship:ship@localhost:5432/ship?sslmode=disable"
	defaultRedisURL    = "redis://localhost:6379/0"
)

type Config struct {
	Environment        string
	Hostname           string
	PublicURL          string
	PublicOrigin       string
	AllowInsecureHTTP  bool
	APIAddr            string
	WorkerAddr         string
	DataDir            string
	DatabaseURL        string
	RedisURL           string
	RunMigrations      bool
	LogLevel           string
	FirstRunTokenHash  string
	SessionSecret      string
	SessionIdleTTL     time.Duration
	SessionAbsoluteTTL time.Duration
	KeyringPath        string
	MasterKey          string
	TrustForwardedIP   bool
}

func Load() (Config, error) {
	runMigrations, err := envBool("SHIP_RUN_MIGRATIONS", false)
	if err != nil {
		return Config{}, err
	}

	allowInsecureHTTP, err := envBool("SHIP_ALLOW_INSECURE_HTTP", false)
	if err != nil {
		return Config{}, err
	}
	trustForwardedIP, err := envBool("SHIP_TRUST_FORWARDED_IP", false)
	if err != nil {
		return Config{}, err
	}
	sessionIdleTTL, err := envDuration("SHIP_SESSION_IDLE_TTL", 24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	sessionAbsoluteTTL, err := envDuration("SHIP_SESSION_ABSOLUTE_TTL", 7*24*time.Hour)
	if err != nil {
		return Config{}, err
	}

	hostname := env("SHIP_HOSTNAME", "localhost")
	publicURL := strings.TrimRight(env("SHIP_PUBLIC_URL", "http://"+hostname+":3000"), "/")
	publicOrigin, secure, err := validatePublicURL(publicURL)
	if err != nil {
		return Config{}, err
	}
	environment := strings.ToLower(env("SHIP_ENV", "development"))
	if environment == "production" && !secure && !allowInsecureHTTP {
		return Config{}, errors.New("SHIP_PUBLIC_URL must use https in production; set SHIP_ALLOW_INSECURE_HTTP=true only for temporary bootstrap access")
	}

	cfg := Config{
		Environment:        environment,
		Hostname:           hostname,
		PublicURL:          publicURL,
		PublicOrigin:       publicOrigin,
		AllowInsecureHTTP:  allowInsecureHTTP,
		APIAddr:            env("SHIP_API_ADDR", ":8080"),
		WorkerAddr:         env("SHIP_WORKER_ADDR", ":8081"),
		DataDir:            env("SHIP_DATA_DIR", "./data/ship"),
		DatabaseURL:        env("DATABASE_URL", defaultDatabaseURL),
		RedisURL:           env("REDIS_URL", defaultRedisURL),
		RunMigrations:      runMigrations,
		LogLevel:           strings.ToLower(env("SHIP_LOG_LEVEL", "info")),
		FirstRunTokenHash:  strings.ToLower(strings.TrimSpace(os.Getenv("SHIP_FIRST_RUN_TOKEN_HASH"))),
		SessionSecret:      strings.TrimSpace(os.Getenv("SHIP_SESSION_SECRET")),
		SessionIdleTTL:     sessionIdleTTL,
		SessionAbsoluteTTL: sessionAbsoluteTTL,
		KeyringPath:        strings.TrimSpace(os.Getenv("SHIP_KEYRING_PATH")),
		MasterKey:          strings.TrimSpace(os.Getenv("SHIP_MASTER_KEY")),
		TrustForwardedIP:   trustForwardedIP,
	}
	if cfg.Environment != "production" && cfg.SessionSecret == "" {
		cfg.SessionSecret = "ship-development-session-secret-do-not-use"
	}
	if cfg.Environment != "production" && cfg.KeyringPath == "" && cfg.MasterKey == "" {
		cfg.MasterKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
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
	if cfg.SessionIdleTTL > cfg.SessionAbsoluteTTL {
		return Config{}, errors.New("SHIP_SESSION_IDLE_TTL must not exceed SHIP_SESSION_ABSOLUTE_TTL")
	}
	if cfg.Environment == "production" && cfg.KeyringPath == "" && cfg.MasterKey == "" {
		return Config{}, errors.New("SHIP_KEYRING_PATH or SHIP_MASTER_KEY is required in production")
	}

	return cfg, nil
}

func (cfg Config) SecureCookies() bool {
	return strings.HasPrefix(cfg.PublicOrigin, "https://")
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

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsed, nil
}

func validatePublicURL(value string) (string, bool, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", false, errors.New("SHIP_PUBLIC_URL must be an absolute http or https URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", false, errors.New("SHIP_PUBLIC_URL must not contain credentials, a path, query, or fragment")
	}
	return parsed.Scheme + "://" + parsed.Host, parsed.Scheme == "https", nil
}
