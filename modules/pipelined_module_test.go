package modules_test

import (
	"testing"

	"github.com/relab/hotstuff/modules"
)

const pipelineCount = 3

type Adder interface {
	Add(a, b int) int
}

type adderImpl struct {
	results []int
}

func NewAdder() *adderImpl { //nolint:revive
	return &adderImpl{
		results: make([]int, 0),
	}
}

func (ad *adderImpl) Add(a, b int) int {
	ad.results = append(ad.results, a+b)
	return a + b
}

type Multiplier interface {
	Mult(a, b int) int
}

type multiplierImpl struct {
	// declares dependencies on other modules
	adder Adder
}

func (m multiplierImpl) Mult(a, b int) int {
	result := a
	for i := 0; i < b; i++ {
		result = m.adder.Add(result, 1)
	}
	return result
}

func NewMultiplier() *multiplierImpl { //nolint:revive
	return &multiplierImpl{}
}

func (m *multiplierImpl) InitModule(mods *modules.Core) {
	// TODO: Figure out some get method
}

func (a *adderImpl) InitModule(mods *modules.Core) {
	// TODO: Figure out some get method
}

const (
	AdderModuleId = iota
	MultiplierModuleId
)

func TestPipelinedModule(t *testing.T) {
	builder := modules.NewBuilder(0, nil, 3)
	builder.CreatePipelined(AdderModuleId, NewAdder)
	builder.CreatePipelined(MultiplierModuleId, NewMultiplier)

	// mods :=
	builder.Build()

	/*var (
		counter Counter
		greeter Greeter
	)

	mods.Get(&counter, &greeter)

	if greeter.Greet("John") != "Hello, John" {
		t.Fail()
	}

	if counter.Count("John") != 1 {
		t.Fail()
	}*/
}
