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
	result := 0
	for i := 0; i < a; i++ {
		result = m.adder.Add(result, b)
	}
	return result
}

func NewMultiplier() *multiplierImpl { //nolint:revive
	return &multiplierImpl{}
}

func (m *multiplierImpl) InitModule(mods *modules.Core) {
	// TODO: Figure out some get method
	pipeId := mods.FindModulePipelineId(m)
	mod, ok := mods.FindModuleFromPipeline(ModuleTypeIdAdder, pipeId)
	if !ok {
		panic("could not find added")
	}

	m.adder = mod.(Adder)
}

func (a *adderImpl) InitModule(mods *modules.Core) {
	// TODO: Figure out some get method
}

const (
	ModuleTypeIdAdder = iota
	ModuleTypeIdMultiplier
)

func TestPipelinedModule(t *testing.T) {
	const pipelineCount = 3
	builder := modules.NewBuilder(0, nil, pipelineCount)
	builder.AddPipelined(ModuleTypeIdAdder, NewAdder)
	builder.AddPipelined(ModuleTypeIdMultiplier, NewMultiplier)

	if builder.PipelineCount() != pipelineCount {
		t.Fail()
	}

	mods := builder.Build()
	multipliers := mods.GetAllPipelinedOfType(ModuleTypeIdMultiplier)
	for _, m := range multipliers {
		m.(*multiplierImpl).Mult(2, 3)
	}
}
