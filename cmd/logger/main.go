package main

import (
	"time"

	"github.com/relab/hotstuff/core/logging"
	"go.uber.org/zap"
)

func main() {
	something := logging.New("test")
	something.Info("Testing...")
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	url := 5.5
	logger.Info("failed to fetch URL",
		// Structured context as strongly typed Field values.
		zap.Float64("url", url),
		zap.Int("attempt", 3),
		zap.Duration("backoff", time.Second),
	)
	something2 := logging.New2("test")
	something2.Infox("Testing...",
		zap.Duration("backoff", time.Second),
	)
}
