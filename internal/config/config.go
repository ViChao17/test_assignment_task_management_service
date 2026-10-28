// Package config загружает конфигурацию из переменных окружения.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr        string
	MySQLDSN        string
	RedisAddr       string
	RedisPassword   string
	RedisDB         int
	JWTSecret       string
	JWTIssuer       string
	JWTTTL          time.Duration
	TaskCacheTTL    time.Duration
	AutoMigrate     bool
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        envOr("HTTP_ADDR", ":8080"),
		MySQLDSN:        os.Getenv("MYSQL_DSN"),
		RedisAddr:       os.Getenv("REDIS_ADDR"),
		RedisPassword:   os.Getenv("REDIS_PASSWORD"),
		RedisDB:         0,
		JWTSecret:       os.Getenv("JWT_SECRET"),
		JWTIssuer:       envOr("JWT_ISSUER", "taskmanager"),
		JWTTTL:          24 * time.Hour,
		TaskCacheTTL:    5 * time.Minute,
		AutoMigrate:     true,
		ShutdownTimeout: 15 * time.Second,
	}

	var err error
	if cfg.RedisDB, err = envInt("REDIS_DB", 0); err != nil {
		return Config{}, err
	}
	if cfg.JWTTTL, err = envDuration("JWT_TTL", cfg.JWTTTL); err != nil {
		return Config{}, err
	}
	if cfg.TaskCacheTTL, err = envDuration("TASK_CACHE_TTL", cfg.TaskCacheTTL); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = envDuration("SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if cfg.AutoMigrate, err = envBool("AUTO_MIGRATE", cfg.AutoMigrate); err != nil {
		return Config{}, err
	}
	if cfg.LogLevel, err = envLogLevel("LOG_LEVEL", slog.LevelInfo); err != nil {
		return Config{}, err
	}

	if strings.TrimSpace(cfg.MySQLDSN) == "" {
		return Config{}, fmt.Errorf("MYSQL_DSN is required")
	}
	if strings.TrimSpace(cfg.RedisAddr) == "" {
		return Config{}, fmt.Errorf("REDIS_ADDR is required")
	}
	if cfg.RedisDB < 0 {
		return Config{}, fmt.Errorf("REDIS_DB must not be negative")
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must contain at least 32 characters")
	}
	if cfg.JWTTTL <= 0 {
		return Config{}, fmt.Errorf("JWT_TTL must be positive")
	}
	if cfg.TaskCacheTTL <= 0 {
		return Config{}, fmt.Errorf("TASK_CACHE_TTL must be positive")
	}
	if cfg.ShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT must be positive")
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration (for example 5m): %w", key, err)
	}
	return parsed, nil
}

func envBool(key string, fallback bool) (bool, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return parsed, nil
}

func envLogLevel(key string, fallback slog.Level) (slog.Level, error) {
	value := strings.ToUpper(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback, nil
	}
	switch value {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARN", "WARNING":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return fallback, fmt.Errorf("%s must be one of DEBUG, INFO, WARN, ERROR", key)
	}
}
