package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.SugaredLogger

func init() {
	atom := zap.NewAtomicLevelAt(zap.DebugLevel)
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.Level = atom
	l, err := config.Build()
	if err != nil {
		panic(err)
	}
	if os.Getenv("HOTSTUFF_LOG") != "1" {
		atom.SetLevel(zap.ErrorLevel)
	}
	logger = l.Sugar()
}

// GetLogger returns a pointer to the global logger for HotStuff
func GetLogger() *zap.SugaredLogger {
	return logger
}
