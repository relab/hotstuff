package modules_test

import (
	"testing"

	"github.com/relab/hotstuff/modules"
)

var TestOption = modules.NewOption()

func TestOptions(t *testing.T) {
	opts := modules.Options{}
	opts.Set(TestOption, 42)
	if opts.Get(TestOption) != 42 {
		t.Error("expected 42, got", opts.Get(TestOption))
	}
}

func BenchmarkOptionsGet(b *testing.B) {
	opts := modules.Options{}
	opts.Set(TestOption, 1)
	for b.Loop() {
		opts.Get(TestOption)
	}
}

func BenchmarkOptionsField(b *testing.B) {
	opts := modules.Options{}
	opts.SetSharedRandomSeed(1)
	for b.Loop() {
		opts.SharedRandomSeed()
	}
}
