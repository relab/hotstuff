package logging

import (
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.SugaredLogger

func init() {
	var config zap.Config
	if strings.ToLower(os.Getenv("HOTSTUFF_LOG_TYPE")) == "json" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
		if isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()) {
			config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		}
	}
	l, err := config.Build()
	if err != nil {
		panic(err)
	}
	switch strings.ToLower(os.Getenv("HOTSTUFF_LOG")) {
	case "1":
		fallthrough
	case "debug":
		config.Level.SetLevel(zap.DebugLevel)
	case "info":
		config.Level.SetLevel(zap.InfoLevel)
	case "warn":
		config.Level.SetLevel(zap.WarnLevel)
	case "error":
		fallthrough
	default:
		config.Level.SetLevel(zap.ErrorLevel)
	}
	logger = l.Sugar()
}

// GetLogger returns a pointer to the global logger for HotStuff
func GetLogger() *zap.SugaredLogger {
	return logger
}
