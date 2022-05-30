package modules_test

import (
	"testing"

	"github.com/relab/hotstuff/modules"
)

func TestModuleRegistry(t *testing.T) {
	modules.RegisterModule("frobulator", func() moduleIface {
		return module{}
	})

	frobulator, ok := modules.GetModule[moduleIface]("frobulator")
	if !ok {
		t.Fatal("module was not found")
	}

	i := 0
	frobulator.frobulate(&i)
	if i != 1 {
		t.Error("module did not behave as expected")
	}
}

type moduleIface interface {
	frobulate(i *int)
}

type module struct{}

func (module) frobulate(i *int) {
	*i++
}
