// Package logging provides structured logging capabilities using zap.Field for type-safe logging.
package logging

import (
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/term"
)

var (
	lokiCfg   *LokiConfig
	lokiMut   sync.RWMutex
	lokiCores []zapcore.Core // track active Loki cores for Sync on shutdown
)

// SetLokiConfig sets the global Loki push configuration.
// When set, all loggers created via New2/New2WithDest will also push logs to Loki.
// Pass nil to disable Loki logging.
func SetLokiConfig(cfg *LokiConfig) {
	lokiMut.Lock()
	defer lokiMut.Unlock()
	lokiCfg = cfg
}

// SyncLoki flushes all buffered Loki log entries. Call this before application exit.
func SyncLoki() {
	lokiMut.RLock()
	defer lokiMut.RUnlock()
	for _, c := range lokiCores {
		_ = c.Sync()
	}
}

// newLokiCoreIfConfigured creates and registers a Loki core if LokiConfig is set.
func newLokiCoreIfConfigured(level zapcore.LevelEnabler) zapcore.Core {
	lokiMut.RLock()
	defer lokiMut.RUnlock()
	if lokiCfg == nil {
		return nil
	}
	core := NewLokiCore(*lokiCfg, level)
	lokiCores = append(lokiCores, core)
	return core
}

// Logger2 provides structured logging with type-safe fields using zap.Field.
// This interface supports structured field logging for common log levels.
type Logger2 interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)

	// Additional convenience methods
	With(fields ...zap.Field) Logger2
	Named(name string) Logger2
}

// wrapper2 implements the Logger2 interface with level management
type wrapper2 struct {
	zapLogger *zap.Logger
	level     zap.AtomicLevel
	mut       sync.Mutex
}

// updateLevel dynamically updates the log level based on package-specific settings
func (wr *wrapper2) updateLevel() {
	var (
		file string
		ok   bool
	)

	mut.RLock()
	defer mut.RUnlock()

	if len(packageLevels) < 1 {
		// no need to do anything
		return
	}

	_, file, _, ok = runtime.Caller(2)

	if ok {
		for k, v := range packageLevels {
			if strings.Contains(file, k) {
				wr.level.SetLevel(v)
				return
			}
		}
	}

	wr.level.SetLevel(logLevel)
}

func (wr *wrapper2) Debug(msg string, fields ...zap.Field) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.zapLogger.Debug(msg, fields...)
}

func (wr *wrapper2) Info(msg string, fields ...zap.Field) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.zapLogger.Info(msg, fields...)
}

func (wr *wrapper2) Warn(msg string, fields ...zap.Field) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.zapLogger.Warn(msg, fields...)
}

func (wr *wrapper2) Error(msg string, fields ...zap.Field) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.zapLogger.Error(msg, fields...)
}

func (wr *wrapper2) With(fields ...zap.Field) Logger2 {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	return &wrapper2{
		zapLogger: wr.zapLogger.With(fields...),
		level:     wr.level,
	}
}

func (wr *wrapper2) Named(name string) Logger2 {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	return &wrapper2{
		zapLogger: wr.zapLogger.Named(name),
		level:     wr.level,
	}
}

// New2 returns a new structured logger for stderr with the given name.
// If a Loki configuration has been set via SetLokiConfig, the logger will
// also push structured log entries to Grafana Loki.
func New2(name string) Logger2 {
	var config zap.Config
	if strings.ToLower(os.Getenv("HOTSTUFF_LOG_TYPE")) == "json" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
		if term.IsTerminal(int(os.Stderr.Fd())) {
			config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		}
	}
	mut.RLock()
	config.Level.SetLevel(logLevel)
	mut.RUnlock()

	opts := []zap.Option{zap.AddCallerSkip(1)}

	// If Loki is configured, wrap the core with a tee to push to both console and Loki.
	if lokiCore := newLokiCoreIfConfigured(config.Level); lokiCore != nil {
		l, err := config.Build()
		if err != nil {
			panic(err)
		}
		teeCore := zapcore.NewTee(l.Core(), lokiCore)
		l = zap.New(teeCore, opts...)
		return &wrapper2{
			zapLogger: l.Named(name),
			level:     config.Level,
		}
	}

	l, err := config.Build(opts...)
	if err != nil {
		panic(err)
	}
	return &wrapper2{
		zapLogger: l.Named(name),
		level:     config.Level,
	}
}

// New2WithDest returns a new structured logger for the given destination with the given name.
// If a Loki configuration has been set via SetLokiConfig, the logger will
// also push structured log entries to Grafana Loki.
func New2WithDest(dest io.Writer, name string) Logger2 {
	atom := zap.NewAtomicLevelAt(logLevel)
	core := zapcore.NewCore(zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()), zapcore.AddSync(dest), atom)

	// If Loki is configured, tee the core.
	if lokiCore := newLokiCoreIfConfigured(atom); lokiCore != nil {
		core = zapcore.NewTee(core, lokiCore)
	}

	l := zap.New(core, zap.AddCallerSkip(1))
	return &wrapper2{
		zapLogger: l.Named(name),
		level:     atom,
	}
}
