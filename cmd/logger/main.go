package main

import (
	"errors"
	"time"

	"github.com/relab/hotstuff/core/logging"
	"go.uber.org/zap"
)

func main() {
	// Original unstructured logging
	something := logging.New("test")
	something.Info("Testing...")

	// Native zap logger (for comparison)
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	url := "http://example.com"
	logger.Info("failed to fetch URL",
		// Structured context as strongly typed Field values.
		zap.String("url", url),
		zap.Int("attempt", 3),
		zap.Duration("backoff", time.Second),
	)

	// Logger2 interface
	structuredLogger := logging.New2("structured-test")

	structuredLogger.Info("Application started",
		zap.String("version", "1.0.0"),
		zap.Int("port", 8080),
		zap.Bool("debug", true),
	)

	structuredLogger.Debug("Processing request",
		zap.String("method", "GET"),
		zap.String("path", "/api/health"),
		zap.Duration("latency", time.Millisecond*150),
	)

	structuredLogger.Warn("High memory usage detected",
		zap.Float64("memory_usage_gb", 7.8),
		zap.Int64("available_gb", 2),
	)

	// Error logging with structured context
	err := errors.New("database connection failed")
	structuredLogger.Error("Failed to connect to database",
		zap.Error(err),
		zap.String("host", "localhost"),
		zap.Int("port", 5432),
		zap.String("database", "hotstuff_db"),
	)

	// Using With() to create a logger with pre-set fields
	userLogger := structuredLogger.With(
		zap.String("user_id", "12345"),
		zap.String("session_id", "abcdef"),
	)

	userLogger.Info("User action performed",
		zap.String("action", "login"),
		zap.Duration("response_time", time.Millisecond*45),
	)

	// Using Named() to create a sub-logger
	dbLogger := structuredLogger.Named("database")
	dbLogger.Info("Query executed",
		zap.String("query", "SELECT * FROM users"),
		zap.Duration("execution_time", time.Millisecond*23),
		zap.Int("rows_affected", 42),
	)
}
