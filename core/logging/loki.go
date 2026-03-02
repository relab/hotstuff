// Package logging provides a Loki integration for structured logging.
// This file implements a zapcore.Core that pushes log entries to Grafana Loki
// via its HTTP push API (/loki/api/v1/push).
package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap/zapcore"
)

// LokiConfig holds configuration for the Loki log aggregation client.
type LokiConfig struct {
	// URL is the Loki push API endpoint, e.g. "http://localhost:3100/loki/api/v1/push".
	URL string
	// Labels are static labels attached to every log stream pushed to Loki.
	// Common labels: {"app": "hotstuff", "node": "replica-1"}.
	Labels map[string]string
	// BatchSize is the maximum number of log entries to buffer before flushing.
	// Default: 100.
	BatchSize int
	// BatchWait is the maximum time to wait before flushing a partial batch.
	// Default: 1 second.
	BatchWait time.Duration
	// TenantID is an optional multi-tenant header for Loki (X-Scope-OrgID).
	TenantID string
}

// lokiPushRequest matches the Loki push API JSON format.
type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// lokiCore implements zapcore.Core and pushes log entries to Loki.
type lokiCore struct {
	cfg     LokiConfig
	encoder zapcore.Encoder
	level   zapcore.LevelEnabler
	fields  []zapcore.Field

	mu      sync.Mutex
	batch   []lokiEntry
	client  *http.Client
	quit    chan struct{}
	done    chan struct{}
	flushed chan struct{}
}

type lokiEntry struct {
	timestamp time.Time
	line      string
}

// NewLokiCore creates a new zapcore.Core that sends logs to Grafana Loki.
// The core buffers log entries and pushes them in batches.
func NewLokiCore(cfg LokiConfig, level zapcore.LevelEnabler) zapcore.Core {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.BatchWait <= 0 {
		cfg.BatchWait = 1 * time.Second
	}
	if cfg.Labels == nil {
		cfg.Labels = map[string]string{"app": "hotstuff"}
	}
	// Ensure "app" label exists.
	if _, ok := cfg.Labels["app"]; !ok {
		cfg.Labels["app"] = "hotstuff"
	}

	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     "",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	lc := &lokiCore{
		cfg:     cfg,
		encoder: zapcore.NewJSONEncoder(encoderCfg),
		level:   level,
		batch:   make([]lokiEntry, 0, cfg.BatchSize),
		client:  &http.Client{Timeout: 5 * time.Second},
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
		flushed: make(chan struct{}),
	}

	go lc.runFlusher()
	return lc
}

// Enabled implements zapcore.Core.
func (lc *lokiCore) Enabled(lvl zapcore.Level) bool {
	return lc.level.Enabled(lvl)
}

// With implements zapcore.Core.
func (lc *lokiCore) With(fields []zapcore.Field) zapcore.Core {
	clone := &lokiCore{
		cfg:     lc.cfg,
		encoder: lc.encoder.Clone(),
		level:   lc.level,
		fields:  append(lc.fields[:len(lc.fields):len(lc.fields)], fields...),
		batch:   lc.batch,
		client:  lc.client,
		quit:    lc.quit,
		done:    lc.done,
		flushed: lc.flushed,
	}
	for _, f := range fields {
		f.AddTo(clone.encoder)
	}
	return clone
}

// Check implements zapcore.Core.
func (lc *lokiCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if lc.Enabled(entry.Level) {
		return ce.AddCore(entry, lc)
	}
	return ce
}

// Write implements zapcore.Core.
func (lc *lokiCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	buf, err := lc.encoder.EncodeEntry(entry, fields)
	if err != nil {
		return err
	}
	line := buf.String()
	buf.Free()

	lc.mu.Lock()
	lc.batch = append(lc.batch, lokiEntry{
		timestamp: entry.Time,
		line:      line,
	})
	shouldFlush := len(lc.batch) >= lc.cfg.BatchSize
	lc.mu.Unlock()

	if shouldFlush {
		lc.flush()
	}
	return nil
}

// Sync implements zapcore.Core. Flushes any remaining buffered logs.
func (lc *lokiCore) Sync() error {
	close(lc.quit)
	<-lc.done
	lc.flush()
	return nil
}

// runFlusher periodically flushes batched log entries to Loki.
func (lc *lokiCore) runFlusher() {
	defer close(lc.done)
	ticker := time.NewTicker(lc.cfg.BatchWait)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lc.flush()
		case <-lc.quit:
			return
		}
	}
}

// flush sends the current batch to Loki and clears it.
func (lc *lokiCore) flush() {
	lc.mu.Lock()
	if len(lc.batch) == 0 {
		lc.mu.Unlock()
		return
	}
	entries := lc.batch
	lc.batch = make([]lokiEntry, 0, lc.cfg.BatchSize)
	lc.mu.Unlock()

	values := make([][]string, len(entries))
	for i, e := range entries {
		values[i] = []string{
			strconv.FormatInt(e.timestamp.UnixNano(), 10),
			e.line,
		}
	}

	payload := lokiPushRequest{
		Streams: []lokiStream{
			{
				Stream: lc.cfg.Labels,
				Values: values,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("loki: failed to marshal push request: %v\n", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, lc.cfg.URL, bytes.NewReader(body))
	if err != nil {
		fmt.Printf("loki: failed to create request: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if lc.cfg.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", lc.cfg.TenantID)
	}

	resp, err := lc.client.Do(req)
	if err != nil {
		fmt.Printf("loki: failed to push logs: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		fmt.Printf("loki: unexpected response status: %s\n", resp.Status)
	}
}
