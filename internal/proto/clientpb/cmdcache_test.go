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
	cache := NewCommandCache(WithBatching(2))

	var wg sync.WaitGroup
	wg.Add(2)

	var want []*Command
	go func() {
		defer wg.Done()
		for i := range 6 {
			cmd := &Command{ClientID: 1, SequenceNumber: uint64(i + 1)}
			want = append(want, cmd)
			cache.Add(cmd)
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
			cache := NewCommandCache(WithBatching(2))

			for _, cmd := range tt.cmds {
				cache.Add(cmd)
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

func TestPreventAddingDuplicates(t *testing.T) {
	tests := []struct {
		name    string
		batchA  *Batch // Batch of commands that have been proposed
		batchB  *Batch // Batch of commands to add to the cache
		wantLen uint32
	}{
		{
			name:    "NoCommands",
			batchA:  &Batch{Commands: nil},
			batchB:  &Batch{Commands: nil},
			wantLen: 0,
		},
		{
			name:    "OneNewCommand",
			batchA:  &Batch{Commands: []*Command{{SequenceNumber: 1}}},
			batchB:  &Batch{Commands: []*Command{{SequenceNumber: 2}}},
			wantLen: 1,
		},
		{
			name:    "OneOldCommand",
			batchA:  &Batch{Commands: []*Command{{SequenceNumber: 1}}},
			batchB:  &Batch{Commands: []*Command{{SequenceNumber: 1}}},
			wantLen: 0,
		},
		{
			name:    "TwoNewCommands",
			batchA:  &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 2}}},
			batchB:  &Batch{Commands: []*Command{{SequenceNumber: 3}, {SequenceNumber: 4}}},
			wantLen: 2,
		},
		{
			name:    "TwoCommandsOneOld",
			batchA:  &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 2}}},
			batchB:  &Batch{Commands: []*Command{{SequenceNumber: 2}, {SequenceNumber: 3}}},
			wantLen: 1, // only the new command is added, the old command is ignored
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewCommandCache(WithBatching(2))

			// mark batchA as proposed; necessary to prevent the cache from adding previously proposed commands
			cache.Proposed(tt.batchA)

			for _, cmd := range tt.batchB.GetCommands() {
				cache.Add(cmd)
			}
			if got := cache.len(); got != tt.wantLen {
				t.Errorf("len() = %d, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestCacheContainsDuplicate(t *testing.T) {
	tests := []struct {
		name   string
		batchA *Batch // Batch to be proposed
		batchB *Batch // Batch to check for duplicates/old commands
		want   bool
	}{
		{
			name:   "NoCommands",
			batchA: &Batch{Commands: nil},
			batchB: &Batch{Commands: nil},
			want:   false, // no commands, no duplicates
		},
		{
			name:   "OneCommandDifferent",
			batchA: &Batch{Commands: []*Command{{SequenceNumber: 1}}},
			batchB: &Batch{Commands: []*Command{{SequenceNumber: 2}}},
			want:   false, // no duplicates; expected behavior
		},
		{
			name:   "OneCommandDuplicate",
			batchA: &Batch{Commands: []*Command{{SequenceNumber: 1}}},
			batchB: &Batch{Commands: []*Command{{SequenceNumber: 1}}},
			want:   true,
		},
		{
			name:   "TwoCommandsDifferent",
			batchA: &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 2}}},
			batchB: &Batch{Commands: []*Command{{SequenceNumber: 3}, {SequenceNumber: 4}}},
			want:   false, // no duplicates; expected behavior
		},
		{
			name:   "TwoCommandsOneDuplicate",
			batchA: &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 2}}},
			batchB: &Batch{Commands: []*Command{{SequenceNumber: 2}, {SequenceNumber: 3}}},
			want:   true,
		},
		{
			name:   "TwoCommandsTwoDuplicates",
			batchA: &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 2}}},
			batchB: &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 2}}},
			want:   true,
		},
		{
			name:   "ThreeCommands",
			batchA: &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 2}, {SequenceNumber: 3}}},
			batchB: &Batch{Commands: []*Command{{SequenceNumber: 4}, {SequenceNumber: 5}, {SequenceNumber: 6}}},
			want:   false, // no duplicates; expected behavior
		},
		{
			name:   "ThreeCommandsOneDuplicate",
			batchA: &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 2}, {SequenceNumber: 3}}},
			batchB: &Batch{Commands: []*Command{{SequenceNumber: 4}, {SequenceNumber: 5}, {SequenceNumber: 1}}},
			want:   true,
		},
		{
			name:   "ThreeCommandsOneDuplicateRepeated",
			batchA: &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 2}, {SequenceNumber: 3}}},
			batchB: &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 1}, {SequenceNumber: 1}}},
			want:   true,
		},
		{
			name:   "ThreeCommandsTwoDuplicates",
			batchA: &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 2}, {SequenceNumber: 3}}},
			batchB: &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 2}, {SequenceNumber: 4}}},
			want:   true,
		},
		{
			name:   "ThreeCommandsAllDuplicates",
			batchA: &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 2}, {SequenceNumber: 3}}},
			batchB: &Batch{Commands: []*Command{{SequenceNumber: 1}, {SequenceNumber: 2}, {SequenceNumber: 3}}},
			want:   true,
		},
		{
			name:   "ThreeOldCommandsDifferentClients",
			batchA: &Batch{Commands: []*Command{{ClientID: 1, SequenceNumber: 5}, {ClientID: 2, SequenceNumber: 10}, {ClientID: 3, SequenceNumber: 20}}},
			batchB: &Batch{Commands: []*Command{{ClientID: 1, SequenceNumber: 1}, {ClientID: 2, SequenceNumber: 2}, {ClientID: 3, SequenceNumber: 3}}},
			want:   true,
		},
		{
			name:   "ThreeNewCommandsDifferentClients",
			batchA: &Batch{Commands: []*Command{{ClientID: 1, SequenceNumber: 5}, {ClientID: 2, SequenceNumber: 10}, {ClientID: 3, SequenceNumber: 20}}},
			batchB: &Batch{Commands: []*Command{{ClientID: 1, SequenceNumber: 6}, {ClientID: 2, SequenceNumber: 11}, {ClientID: 3, SequenceNumber: 21}}},
			want:   false, // no duplicates; expected behavior
		},
		{
			name:   "ThreeNewCommandsDifferentClientsJumpSequenceNumbers",
			batchA: &Batch{Commands: []*Command{{ClientID: 1, SequenceNumber: 5}, {ClientID: 2, SequenceNumber: 10}, {ClientID: 3, SequenceNumber: 20}}},
			batchB: &Batch{Commands: []*Command{{ClientID: 1, SequenceNumber: 7}, {ClientID: 2, SequenceNumber: 11}, {ClientID: 3, SequenceNumber: 21}}},
			want:   false, // no duplicates, but client 1 skipped sequence number 6; TODO(meling): Is this expected/allowed behavior?
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewCommandCache(WithBatching(2))

			// mark batchA as proposed
			cache.Proposed(tt.batchA)

			// check if batchB contains duplicates with respect to batchA
			if got := cache.ContainsDuplicate(tt.batchB); got != tt.want {
				t.Errorf("ContainsDuplicate() = %v, want %v", got, tt.want)
			}
		})
	}
}
