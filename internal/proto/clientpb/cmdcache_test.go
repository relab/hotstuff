package clientpb

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAddGetCommand(t *testing.T) {
	cache := New(WithBatching(2))

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		// TODO(meling): This should be 6, but there is a probably a off-by-one error in the cache.
		for i := range 7 {
			cmd := &Command{ClientID: 1, SequenceNumber: uint64(i + 1)}
			cache.Add(cmd)
			t.Logf("Added command: %v", cmd)
		}
	}()

	go func() {
		defer wg.Done()
		for range 3 {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			cmd, err := cache.Get(ctx)
			if err != nil {
				t.Errorf("Get() error: %v", err)
			}
			batch, err := cache.getCommands(cmd)
			if err != nil {
				t.Errorf("Batch() error: %v", err)
			}
			t.Logf("Received command: %v", batch)
		}
	}()
	wg.Wait()
}
