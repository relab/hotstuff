// Package logging defines the Logger interface which is used by the module system.
// It also includes functions for setting the global log level and a per-package log level.
package logging

import (
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/mattn/go-isatty"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logLevel      zapcore.Level
	packageLevels = make(map[string]zapcore.Level)
	mut           sync.RWMutex
)

func parseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zap.DebugLevel
	case "info":
		return zap.InfoLevel
	case "warn":
		return zap.WarnLevel
	case "error":
		return zap.ErrorLevel
	case "panic":
		return zap.PanicLevel
	case "fatal":
		return zap.FatalLevel
	default:
		panic("invalid log level '" + level + "'")
	}
}

// SetLogLevel sets the global log level.
func SetLogLevel(levelStr string) {
	level := parseLevel(levelStr)
	mut.Lock()
	logLevel = level
	mut.Unlock()
}

// SetPackageLogLevel sets a log level for a package, overriding the global level.
func SetPackageLogLevel(packageName, levelStr string) {
	level := parseLevel(levelStr)
	mut.Lock()
	packageLevels[packageName] = level
	mut.Unlock()
}

// Logger is the logging interface used by consensus. It is based on zap.SugaredLogger
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

type wrapper struct {
	inner Logger
	level zap.AtomicLevel
	mut   sync.Mutex
}

func (wr *wrapper) updateLevel() {
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

func (wr *wrapper) DPanic(args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.DPanic(args...)
}

func (wr *wrapper) DPanicf(template string, args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.DPanicf(template, args...)
}

func (wr *wrapper) Debug(args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.Debug(args...)
}

func (wr *wrapper) Debugf(template string, args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.Debugf(template, args...)
}

func (wr *wrapper) Error(args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.Error(args...)
}

func (wr *wrapper) Errorf(template string, args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.Errorf(template, args...)
}

func (wr *wrapper) Fatal(args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.Fatal(args...)
}

func (wr *wrapper) Fatalf(template string, args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.Fatalf(template, args...)
}

func (wr *wrapper) Info(args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.Info(args...)
}

func (wr *wrapper) Infof(template string, args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.Infof(template, args...)
}

func (wr *wrapper) Panic(args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.Panic(args...)
}

func (wr *wrapper) Panicf(template string, args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.Panicf(template, args...)
}

func (wr *wrapper) Warn(args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.Warn(args...)
}

func (wr *wrapper) Warnf(template string, args ...interface{}) {
	wr.mut.Lock()
	defer wr.mut.Unlock()
	wr.updateLevel()
	wr.inner.Warnf(template, args...)
}

// New returns a new logger for stderr with the given name.
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
	mut.RLock()
	config.Level.SetLevel(logLevel)
	mut.RUnlock()
	l, err := config.Build(zap.AddCallerSkip(1))
	if err != nil {
		panic(err)
	}
	return &wrapper{inner: l.Sugar().Named(name), level: config.Level}
}

// NewWithDest returns a new logger for the given destination with the given name.
func NewWithDest(dest io.Writer, name string) Logger {
	atom := zap.NewAtomicLevelAt(logLevel)
	core := zapcore.NewCore(zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()), zapcore.AddSync(dest), atom)
	l := zap.New(core, zap.AddCallerSkip(1))
	return &wrapper{inner: l.Sugar().Named(name), level: atom}
}
