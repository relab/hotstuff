package logging

import (
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is the logging interface used by hotstuff. It is based on zap.SugaredLogger
type Logger interface {
	DPanic(args ...interface{})
	DPanicf(template string, args ...interface{})
	Debug(args ...interface{})
	Debugf(template string, args ...interface{})
	Error(args ...interface{})
	Errorf(template string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(template string, args ...interface{})
	Info(args ...interface{})
	Infof(template string, args ...interface{})
	Panic(args ...interface{})
	Panicf(template string, args ...interface{})
	Warn(args ...interface{})
	Warnf(template string, args ...interface{})
}

// New returns a new logger with the given name.
func New(name string) Logger {
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
	return l.Sugar().Named(name)
}
