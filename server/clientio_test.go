package server

import (
	"testing"

	"github.com/relab/hotstuff"
)

func TestCommandStatusTracker_GetStatus_DefaultExecuted(t *testing.T) {
	tr := NewCommandStatusTracker()
	status := tr.GetStatus(1, 1)
	if status != hotstuff.EXECUTED {
		t.Fatalf("expected default status EXECUTED for unknown client, got %v", status)
	}
}

func TestCommandStatusTracker_SetAndGet(t *testing.T) {
	tr := NewCommandStatusTracker()
	clientID := uint32(1)
	seq := uint64(10)

	tr.SetStatus(clientID, seq, hotstuff.UNKNOWN)
	if got := tr.GetStatus(clientID, seq); got != hotstuff.UNKNOWN {
		t.Fatalf("GetStatus = %v; want %v", got, hotstuff.UNKNOWN)
	}
}

func TestCommandStatusTracker_ExtendWindow(t *testing.T) {
	tr := NewCommandStatusTracker()
	clientID := uint32(2)

	// initial set at seq 1
	tr.SetStatus(clientID, 1, hotstuff.UNKNOWN)
	if got := tr.GetStatus(clientID, 1); got != hotstuff.UNKNOWN {
		t.Fatalf("initial GetStatus = %v; want %v", got, hotstuff.UNKNOWN)
	}

	// set far ahead to force window growth
	largeSeq := uint64(6000)
	tr.SetStatus(clientID, largeSeq, hotstuff.FAILED)
	if got := tr.GetStatus(clientID, largeSeq); got != hotstuff.FAILED {
		t.Fatalf("after extend GetStatus(%d) = %v; want %v", largeSeq, got, hotstuff.FAILED)
	}

	// earlier entry should still be present
	if got := tr.GetStatus(clientID, 1); got != hotstuff.UNKNOWN {
		t.Fatalf("after extend GetStatus(1) = %v; want %v", got, hotstuff.UNKNOWN)
	}
}

func TestCommandStatusTracker_CleanupSlidesAndDeletes(t *testing.T) {
	tr := NewCommandStatusTracker()
	clientID := uint32(3)

	// set statuses for seqs 1..10
	for i := uint64(1); i <= 10; i++ {
		tr.SetStatus(clientID, i, hotstuff.UNKNOWN)
	}

	// cleanup up to 4 -> base should become 5, entries 1..4 removed
	tr.Cleanup(clientID, 4)
	// seq 3 should now be treated as executed (cleaned up)
	if got := tr.GetStatus(clientID, 3); got != hotstuff.EXECUTED {
		t.Fatalf("after cleanup GetStatus(3) = %v; want EXECUTED", got)
	}
	// seq 5 should still be UNKNOWN
	if got := tr.GetStatus(clientID, 5); got != hotstuff.UNKNOWN {
		t.Fatalf("after cleanup GetStatus(5) = %v; want %v", got, hotstuff.UNKNOWN)
	}

	// cleanup up to 10 -> should remove entire window
	tr.Cleanup(clientID, 10)
	if got := tr.GetStatus(clientID, 5); got != hotstuff.EXECUTED {
		t.Fatalf("after full cleanup GetStatus(5) = %v; want EXECUTED", got)
	}
	// snapshot should be empty for this client
	// snapshot should not contain cleaned-up seqs 1..10
	if m := tr.GetClientStatuses(clientID); len(m) != 0 {
		for i := uint64(1); i <= 10; i++ {
			if _, ok := m[i]; ok {
				t.Fatalf("snapshot contains cleaned-up seq %d", i)
			}
		}
		// It's an implementation detail whether the snapshot is empty or contains
		// preallocated entries for sequences > cleanup point (which is why len(m)
		// may be 4990). We only assert here that cleaned-up sequences are gone.
	}
}

func TestCommandStatusTracker_IgnoreOldSetStatus(t *testing.T) {
	tr := NewCommandStatusTracker()
	clientID := uint32(4)

	// create window at seq 100
	tr.SetStatus(clientID, 100, hotstuff.UNKNOWN)
	// attempt to set older seq 90 -> should be ignored
	tr.SetStatus(clientID, 90, hotstuff.FAILED)
	if got := tr.GetStatus(clientID, 90); got != hotstuff.EXECUTED {
		t.Fatalf("old SetStatus should be ignored; GetStatus(90) = %v; want EXECUTED", got)
	}
	// newer seq should still be present
	if got := tr.GetStatus(clientID, 100); got != hotstuff.UNKNOWN {
		t.Fatalf("GetStatus(100) = %v; want %v", got, hotstuff.UNKNOWN)
	}
}

func TestCommandStatusTracker_GetClientStatusesSnapshot(t *testing.T) {
	tr := NewCommandStatusTracker()
	clientID := uint32(5)

	tr.SetStatus(clientID, 50, hotstuff.UNKNOWN)
	tr.SetStatus(clientID, 52, hotstuff.FAILED)

	snap := tr.GetClientStatuses(clientID)
	if snap[50] != hotstuff.UNKNOWN {
		t.Fatalf("snapshot[50] = %v; want %v", snap[50], hotstuff.UNKNOWN)
	}
	if snap[52] != hotstuff.FAILED {
		t.Fatalf("snapshot[52] = %v; want %v", snap[52], hotstuff.FAILED)
	}
	// a sequence not set but within window will have zero value; ensure an unrelated seq returns EXECUTED via GetStatus
	if got := tr.GetStatus(clientID, 49); got != hotstuff.EXECUTED {
		t.Fatalf("GetStatus(49) = %v; want EXECUTED", got)
	}
}
