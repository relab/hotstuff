package modules_test

import (
	"testing"

	"github.com/relab/hotstuff/modules"
)

type Adder interface {
	Add(a, b int) int
	LastResult() int
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

func (ad *adderImpl) LastResult() int {
	return ad.results[len(ad.results)-1]
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
	if !mods.TryGetPipelined(m, &m.adder) {
		panic("adder dependency could not be found")
	}
}

func (a *adderImpl) InitModule(_ *modules.Core) {
	// Does nothing for now
}

func TestPipelinedModule(t *testing.T) {
	const pipelineCount = 3
	builder := modules.NewBuilder(0, nil, pipelineCount)
	builder.EmplacePipelined(NewAdder)
	builder.EmplacePipelined(NewMultiplier)

	if builder.PipelineCount() != pipelineCount {
		t.Fail()
	}

	builder.Build()

	type AdderMultTestCase struct {
		A      int
		B      int
		Result int
	}

	// Currently, pipeline ids are ints that get incremented from 0.
	testCasesMult := map[modules.PipelineId]AdderMultTestCase{
		0: {A: 2, B: 3, Result: 6},
		1: {A: 2, B: 5, Result: 10},
		2: {A: 2, B: 6, Result: 12},
	}

	pipeIds := builder.GetPipelineIds()
	for _, id := range pipeIds {
		pipeline := builder.GetPipeline(id)
		multer := pipeline[1].(Multiplier)
		tc := testCasesMult[id]
		actualResult := multer.Mult(tc.A, tc.B)
		if tc.Result != actualResult {
			t.Fail()
		}

		// The last result stored in the adder is the same as the result of multiplier,
		// since the multiple addings will add up to the multiplication answer.
		adder := pipeline[0].(Adder)
		if adder.LastResult() != tc.Result || adder.LastResult() != actualResult {
			t.Fail()
		}
	}
}
