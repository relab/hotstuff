package clientpb

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestCacheConcurrentAddGet(t *testing.T) {
	cache := New(WithBatching(2))

	var wg sync.WaitGroup
	wg.Add(2)

	var want []*Command
	go func() {
		defer wg.Done()
		for i := range 6 {
			cmd := &Command{ClientID: 1, SequenceNumber: uint64(i + 1)}
			want = append(want, cmd)
			cache.add(cmd)
		}
	}()

	go func() {
		defer wg.Done()

		var got []*Command
		for range 3 {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			batch, err := cache.Get(ctx)
			if err != nil {
				t.Errorf("Get() error: %v", err)
			}
			cmds := batch.GetCommands()
			got = append(got, cmds...)
			if len(cmds) != 2 {
				t.Errorf("Get() got %d commands, want 2", len(cmds))
			}
		}
		if diff := cmp.Diff(got, want, protocmp.Transform()); diff != "" {
			t.Errorf("Get() mismatch (-got +want):\n%s", diff)
		}
	}()
	wg.Wait()
}

func TestCacheAddGetDeadlineExceeded(t *testing.T) {
	tests := []struct {
		name      string
		cmds      []*Command
		wantBatch [][]*Command
		wantErr   []error
	}{
		{
			name:      "NoCommands/DeadlineExceeded",
			cmds:      nil,
			wantBatch: [][]*Command{nil},
			wantErr:   []error{context.DeadlineExceeded},
		},
		{
			name:      "OneCommand/DeadlineExceeded",
			cmds:      []*Command{{ClientID: 1, SequenceNumber: 1}},
			wantBatch: [][]*Command{nil},
			wantErr:   []error{context.DeadlineExceeded},
		},
		{
			name: "TwoCommands",
			cmds: []*Command{{ClientID: 1, SequenceNumber: 1}, {ClientID: 1, SequenceNumber: 2}},
			wantBatch: [][]*Command{
				{{ClientID: 1, SequenceNumber: 1}, {ClientID: 1, SequenceNumber: 2}},
			},
			wantErr: []error{nil},
		},
		{
			name: "ThreeCommands/DeadlineExceeded",
			cmds: []*Command{
				{ClientID: 1, SequenceNumber: 1},
				{ClientID: 1, SequenceNumber: 2},
				{ClientID: 1, SequenceNumber: 3},
			},
			wantBatch: [][]*Command{
				{{ClientID: 1, SequenceNumber: 1}, {ClientID: 1, SequenceNumber: 2}},
				{},
			},
			wantErr: []error{nil, context.DeadlineExceeded},
		},
		{
			name: "FourCommands",
			cmds: []*Command{
				{ClientID: 1, SequenceNumber: 1},
				{ClientID: 1, SequenceNumber: 2},
				{ClientID: 1, SequenceNumber: 3},
				{ClientID: 1, SequenceNumber: 4},
			},
			wantBatch: [][]*Command{
				{{ClientID: 1, SequenceNumber: 1}, {ClientID: 1, SequenceNumber: 2}},
				{{ClientID: 1, SequenceNumber: 3}, {ClientID: 1, SequenceNumber: 4}},
			},
			wantErr: []error{nil, nil},
		},
		{
			name: "FiveCommands/DeadlineExceeded",
			cmds: []*Command{
				{ClientID: 1, SequenceNumber: 1},
				{ClientID: 1, SequenceNumber: 2},
				{ClientID: 1, SequenceNumber: 3},
				{ClientID: 1, SequenceNumber: 4},
				{ClientID: 1, SequenceNumber: 5},
			},
			wantBatch: [][]*Command{
				{{ClientID: 1, SequenceNumber: 1}, {ClientID: 1, SequenceNumber: 2}},
				{{ClientID: 1, SequenceNumber: 3}, {ClientID: 1, SequenceNumber: 4}},
				{},
			},
			wantErr: []error{nil, nil, context.DeadlineExceeded},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := New(WithBatching(2))

			for _, cmd := range tt.cmds {
				cache.add(cmd)
			}

			for e := range tt.wantErr {
				wantBatch := tt.wantBatch[e]
				wantErr := tt.wantErr[e]

				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				defer cancel()

				got, err := cache.Get(ctx)
				if (err != nil) != (wantErr != nil) || (wantErr != nil && !errors.Is(err, wantErr)) {
					t.Errorf("Get() error = %v, wantErr %v", err, wantErr)
					t.Logf("Got command batch: %v", got.GetCommands())
					return
				}

				if len(wantBatch) > 0 {
					// we use GetCommands to unmarshal the commands and confirm they match the expected commands
					gotBatch := got.GetCommands()
					if diff := cmp.Diff(gotBatch, wantBatch, protocmp.Transform()); diff != "" {
						t.Errorf("Get() mismatch (-got +want):\n%s", diff)
					}
				}
			}
		})
	}
}
