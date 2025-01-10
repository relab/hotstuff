package core_test

import (
	"testing"

	"github.com/relab/hotstuff/core"
)

var TestOption = core.NewOption()

func TestOptions(t *testing.T) {
	opts := core.Options{}
	opts.Set(TestOption, 42)
	if opts.Get(TestOption) != 42 {
		t.Error("expected 42, got", opts.Get(TestOption))
	}
}

func BenchmarkOptionsGet(b *testing.B) {
	opts := core.Options{}
	var v any
	opts.Set(TestOption, 1)
	for i := 0; i < b.N; i++ {
		v = opts.Get(TestOption).(int)
	}
	_ = v
}

func BenchmarkOptionsField(b *testing.B) {
	opts := core.Options{}
	var v any
	opts.SetSharedRandomSeed(1)
	for i := 0; i < b.N; i++ {
		v = opts.SharedRandomSeed()
	}
	_ = v
}
