package viewduration

import (
	"testing"
	"time"
)

func checkDuration(t *testing.T, funcName string, want, got time.Duration) {
	if want != got {
		t.Fatalf("incorrect view duration after calling %s (want: %d, got: %d)", funcName, want, got)
	}
}

func TestFixed(t *testing.T) {
	want := 100 * time.Microsecond
	vd := NewFixed(want)
	checkDuration(t, "nothing", want, vd.Duration())
	vd.ViewStarted()
	checkDuration(t, "ViewStarted", want, vd.Duration())
	vd.ViewSucceeded()
	checkDuration(t, "ViewSucceeded", want, vd.Duration())
	vd.ViewTimeout()
	checkDuration(t, "ViewTimeout", want, vd.Duration())
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
	checkDuration(t, "nothing", startTimeout, vd.Duration())
	vd.ViewStarted()
	checkDuration(t, "ViewStarted", startTimeout, vd.Duration())
	vd.ViewTimeout()
	checkDuration(t, "ViewTimeout", time.Duration(multiplier)*startTimeout, vd.Duration())
	// timeout many times to reach max timeout
	for range 10 {
		vd.ViewTimeout()
	}
	checkDuration(t, "ViewTimeout 10 times", maxTimeout, vd.Duration())
	time.Sleep(2 * time.Second)
	vd.ViewSucceeded()
	if vd.Duration() == 0 {
		t.Fatal("expected view duration to be greater than zero")
	}
}
