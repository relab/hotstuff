package logging

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestLokiCorePushesLogs(t *testing.T) {
	var (
		mu       sync.Mutex
		received []lokiPushRequest
	)

	// Start a mock Loki server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json content-type, got %s", ct)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var req lokiPushRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("failed to unmarshal request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mu.Lock()
		received = append(received, req)
		mu.Unlock()

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg := LokiConfig{
		URL:       srv.URL,
		Labels:    map[string]string{"app": "hotstuff-test", "env": "test"},
		BatchSize: 2,
		BatchWait: 50 * time.Millisecond,
	}

	core := NewLokiCore(cfg, zapcore.DebugLevel)
	logger := zap.New(core)

	// Write enough logs to trigger a batch flush (BatchSize=2).
	logger.Info("first message", zap.String("key", "val1"))
	logger.Info("second message", zap.String("key", "val2"))

	// Give the flusher time to send.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("expected at least one push request to Loki, got none")
	}

	// Verify the stream labels and values.
	found := false
	for _, req := range received {
		for _, stream := range req.Streams {
			if stream.Stream["app"] == "hotstuff-test" && stream.Stream["env"] == "test" {
				found = true
				if len(stream.Values) < 1 {
					t.Error("expected at least 1 log value in stream")
				}
			}
		}
	}
	if !found {
		t.Error("did not find stream with expected labels")
	}

	// Sync (stop flusher) should not panic.
	_ = core.Sync()
}

func TestLokiCoreTenantHeader(t *testing.T) {
	var gotTenant string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant = r.Header.Get("X-Scope-OrgID")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg := LokiConfig{
		URL:       srv.URL,
		Labels:    map[string]string{"app": "hotstuff"},
		TenantID:  "my-tenant",
		BatchSize: 1,
		BatchWait: 50 * time.Millisecond,
	}

	core := NewLokiCore(cfg, zapcore.DebugLevel)
	logger := zap.New(core)
	logger.Info("tenant test")

	time.Sleep(200 * time.Millisecond)

	if gotTenant != "my-tenant" {
		t.Errorf("expected tenant header 'my-tenant', got %q", gotTenant)
	}

	_ = core.Sync()
}

func TestLokiBatchWaitFlush(t *testing.T) {
	var (
		mu       sync.Mutex
		received int
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req lokiPushRequest
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		for _, s := range req.Streams {
			received += len(s.Values)
		}
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg := LokiConfig{
		URL:       srv.URL,
		Labels:    map[string]string{"app": "hotstuff"},
		BatchSize: 1000, // High batch size so it won't trigger size-based flush.
		BatchWait: 100 * time.Millisecond,
	}

	core := NewLokiCore(cfg, zapcore.DebugLevel)
	logger := zap.New(core)

	// Send a single log that won't fill the batch.
	logger.Info("timer flush test")

	// Wait for the batch timer to fire.
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if received < 1 {
		t.Errorf("expected at least 1 log flushed by timer, got %d", received)
	}

	_ = core.Sync()
}

func TestSetLokiConfigIntegration(t *testing.T) {
	var (
		mu       sync.Mutex
		received int
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req lokiPushRequest
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		for _, s := range req.Streams {
			received += len(s.Values)
		}
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	// Configure Loki globally.
	SetLokiConfig(&LokiConfig{
		URL:       srv.URL,
		Labels:    map[string]string{"app": "hotstuff", "test": "integration"},
		BatchSize: 1,
		BatchWait: 50 * time.Millisecond,
	})
	defer SetLokiConfig(nil)

	// Create a Logger2 — it should automatically wire up Loki.
	logger := New2WithDest(io.Discard, "test-loki")
	logger.Info("hello loki", zap.String("foo", "bar"))

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if received < 1 {
		t.Errorf("expected Logger2 to push to Loki, got %d entries", received)
	}

	SyncLoki()
}
