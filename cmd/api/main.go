package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"taskmanager/internal/api"
	"taskmanager/internal/auth"
	"taskmanager/internal/cache"
	"taskmanager/internal/config"
	"taskmanager/internal/db"
	"taskmanager/internal/service"
	"taskmanager/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startupCtx, cancelStartup := context.WithTimeout(rootCtx, 60*time.Second)
	defer cancelStartup()

	database, err := db.Open(startupCtx, cfg.MySQLDSN, 30*time.Second)
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Warn("failed to close mysql", "error", err)
		}
	}()

	if cfg.AutoMigrate {
		if err := db.Migrate(startupCtx, database); err != nil {
			return fmt.Errorf("apply migrations: %w", err)
		}
		logger.Info("database migrations applied")
	}

	taskCache := cache.NewRedisTaskCache(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.TaskCacheTTL)
	defer func() {
		if err := taskCache.Close(); err != nil {
			logger.Warn("failed to close redis", "error", err)
		}
	}()
	pingCtx, cancelPing := context.WithTimeout(startupCtx, 3*time.Second)
	if err := taskCache.Ping(pingCtx); err != nil {
		logger.Warn("redis is unavailable at startup; task lists will use mysql until it recovers", "error", err)
	} else {
		logger.Info("connected to redis")
	}
	cancelPing()

	storeInstance := store.New(database)
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTTTL)
	authService := service.NewAuthService(storeInstance, tokens)
	teamService := service.NewTeamService(storeInstance)
	taskService := service.NewTaskService(storeInstance, taskCache, logger)
	statsService := service.NewStatsService(storeInstance)

	server := api.NewServer(
		authService,
		teamService,
		taskService,
		statsService,
		tokens,
		storeInstance,
		taskCache,
		logger,
	)

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	listenErr := make(chan error, 1)
	go func() {
		logger.Info("http server started", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErr <- err
			return
		}
		listenErr <- nil
	}()

	select {
	case <-rootCtx.Done():
		logger.Info("shutdown signal received")
	case err := <-listenErr:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancelShutdown()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("http server stopped")
	return nil
}
