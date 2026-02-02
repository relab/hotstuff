package server

import (
	"testing"
)

func TestCommandStatusTracker_SetSuccess(t *testing.T) {
	tr := NewCommandStatusTracker()
	clientID := uint32(1)

	tr.setSuccess(clientID, 10)
	w := tr.GetClientStatuses(clientID)

	if w.HighestSuccess != 10 {
		t.Fatalf("HighestSuccess = %d; want 10", w.HighestSuccess)
	}

	// Setting lower sequence number should not update
	tr.setSuccess(clientID, 5)
	w = tr.GetClientStatuses(clientID)
	if w.HighestSuccess != 10 {
		t.Fatalf("HighestSuccess = %d; want 10", w.HighestSuccess)
	}

	// Setting higher should update
	tr.setSuccess(clientID, 20)
	w = tr.GetClientStatuses(clientID)
	if w.HighestSuccess != 20 {
		t.Fatalf("HighestSuccess = %d; want 20", w.HighestSuccess)
	}
}

func TestCommandStatusTracker_AddFailed(t *testing.T) {
	tr := NewCommandStatusTracker()
	tr.maxFailed = 3 // Set small limit for testing
	clientID := uint32(2)

	tr.addFailed(clientID, 10)
	tr.addFailed(clientID, 15)
	tr.addFailed(clientID, 20)

	w := tr.GetClientStatuses(clientID)
	if len(w.FailedCmds) != 3 {
		t.Fatalf("FailedCmds length = %d; want 3", len(w.FailedCmds))
	}

	// Adding one more should drop the oldest
	tr.addFailed(clientID, 25)
	w = tr.GetClientStatuses(clientID)
	if len(w.FailedCmds) != 3 {
		t.Fatalf("FailedCmds length = %d; want 3", len(w.FailedCmds))
	}

	// Should contain 15, 20, 25 (10 should be dropped)
	expected := []uint64{15, 20, 25}
	for i, seq := range w.FailedCmds {
		if seq != expected[i] {
			t.Fatalf("FailedCmds[%d] = %d; want %d", i, seq, expected[i])
		}
	}
}

func TestCommandStatusTracker_GetClientStatuses(t *testing.T) {
	tr := NewCommandStatusTracker()
	clientID := uint32(3)

	// New client should have empty window
	w := tr.GetClientStatuses(clientID)
	if w.HighestSuccess != 0 {
		t.Fatalf("HighestSuccess = %d; want 0", w.HighestSuccess)
	}
	if len(w.FailedCmds) != 0 {
		t.Fatalf("FailedCmds length = %d; want 0", len(w.FailedCmds))
	}

	// Add some data
	tr.setSuccess(clientID, 50)
	tr.addFailed(clientID, 52)
	tr.addFailed(clientID, 55)

	w = tr.GetClientStatuses(clientID)
	if w.HighestSuccess != 50 {
		t.Fatalf("HighestSuccess = %d; want 50", w.HighestSuccess)
	}
	if len(w.FailedCmds) != 2 {
		t.Fatalf("FailedCmds length = %d; want 2", len(w.FailedCmds))
	}
}
