package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/testforge/testforge/internal/api"
	"github.com/testforge/testforge/internal/config"
	"github.com/testforge/testforge/internal/repository/postgres"
	rediscache "github.com/testforge/testforge/internal/repository/redis"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger := initLogger(cfg.App.Environment, cfg.App.LogLevel)
	defer logger.Sync()

	logger.Info("Starting TestForge API",
		zap.String("version", cfg.App.Version),
		zap.String("environment", cfg.App.Environment),
	)

	// Connect to PostgreSQL
	db, err := postgres.New(cfg.Database)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()
	logger.Info("Connected to PostgreSQL",
		zap.String("host", cfg.Database.Host),
		zap.Int("port", cfg.Database.Port),
	)

	// Connect to Redis (optional)
	var cache *rediscache.Cache
	cache, err = rediscache.New(cfg.Redis)
	if err != nil {
		logger.Warn("Failed to connect to Redis, caching disabled", zap.Error(err))
		cache = nil
	} else {
		defer cache.Close()
		logger.Info("Connected to Redis", zap.String("addr", cfg.Redis.Addr()))
	}

	// Connect to Temporal (optional but recommended)
	var temporalClient client.Client
	temporalClient, err = client.Dial(client.Options{
		HostPort:  cfg.Temporal.Address(),
		Namespace: cfg.Temporal.Namespace,
	})
	if err != nil {
		logger.Warn("Failed to connect to Temporal, workflow execution disabled", zap.Error(err))
		temporalClient = nil
	} else {
		defer temporalClient.Close()
		logger.Info("Connected to Temporal",
			zap.String("address", cfg.Temporal.Address()),
			zap.String("namespace", cfg.Temporal.Namespace),
		)
	}

	// Initialize repositories
	repos := postgres.NewRepositories(db.DB)

	// Create router
	router := api.NewRouter(api.RouterConfig{
		Repos:          repos,
		Cache:          cache,
		TemporalClient: temporalClient,
		TaskQueue:      cfg.Temporal.TaskQueue,
		Logger:         logger,
		EnableCORS:     true,
		RateLimit:      300, // requests per minute
		Development:    cfg.IsDevelopment(),
	})

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("API server listening", zap.String("addr", addr))
		serverErrors <- server.ListenAndServe()
	}()

	// Wait for shutdown signal or server error
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Fatal("Server error", zap.Error(err))

	case sig := <-shutdown:
		logger.Info("Shutdown signal received", zap.String("signal", sig.String()))

		// Create shutdown context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
		defer cancel()

		// Attempt graceful shutdown
		if err := server.Shutdown(ctx); err != nil {
			logger.Error("Graceful shutdown failed, forcing close", zap.Error(err))
			server.Close()
		}

		logger.Info("Server stopped gracefully")
	}
}

// initLogger creates a configured zap logger
func initLogger(env, level string) *zap.Logger {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	var config zap.Config
	if env == "production" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	config.Level = zap.NewAtomicLevelAt(zapLevel)

	logger, err := config.Build()
	if err != nil {
		// Fall back to basic logger
		logger, _ = zap.NewProduction()
	}

	return logger
}
