package viewduration

import (
	"testing"
	"time"
)

func checkDuration(t *testing.T, want, got time.Duration) {
	if want != got {
		t.Fatalf("incorrect fixed view duration (want: %d, got: %d)", want, got)
	}
}

func TestFixed(t *testing.T) {
	want := 100 * time.Microsecond
	vd := NewFixed(want)
	checkDuration(t, want, vd.Duration())
	vd.ViewStarted()
	checkDuration(t, want, vd.Duration())
	vd.ViewSucceeded()
	checkDuration(t, want, vd.Duration())
	vd.ViewTimeout()
	checkDuration(t, want, vd.Duration())
}

func TestDynamic(t *testing.T) {
	sampleSize := uint32(5)
	startTimeout := 100 * time.Millisecond
	maxTimeout := 500 * time.Millisecond
	multiplier := float32(2)
	params := NewParams(
		sampleSize,
		startTimeout,
		maxTimeout,
		multiplier,
	)
	vd := NewDynamic(params)
	checkDuration(t, startTimeout, vd.Duration())
	vd.ViewStarted()
	checkDuration(t, startTimeout, vd.Duration())
	vd.ViewTimeout()
	checkDuration(t, time.Duration(multiplier)*startTimeout, vd.Duration())
	// timeout many times to reach max timeout
	for range 10 {
		vd.ViewTimeout()
	}
	checkDuration(t, maxTimeout, vd.Duration())
	vd.ViewSucceeded()
	checkDuration(t, 0, vd.Duration())
}
