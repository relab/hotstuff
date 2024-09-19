package modules_test

import (
	"testing"

	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/pipeline"
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

func (m *multiplierImpl) Mult(a, b int) int {
	result := 0
	for i := 0; i < a; i++ {
		result = m.adder.Add(result, b)
	}
	return result
}

func NewMultiplier() *multiplierImpl { //nolint:revive
	return &multiplierImpl{}
}

func (m *multiplierImpl) InitModule(mods *modules.Core, _ modules.InitOptions) {
	mods.GetFromPipe(m, &m.adder) // Requires an adder from the same pipe
}

// func (a *adderImpl) InitModule(_ *modules.Core, buildOpt modules.InitOptions) {
// 	// Does nothing for now
// }

func TestPipeliningDisabled(t *testing.T) {
	builder := modules.NewBuilder(0, nil)

	builder.AddPiped(NewAdder)
	builder.AddPiped(NewMultiplier)

	mods := builder.Build()
	if mods.PipeCount() > 0 {
		t.Fail()
	}

	var adder Adder
	var multiplier Multiplier
	mods.Get(&adder, &multiplier)

	result := multiplier.Mult(2, 3)
	if result != 6 {
		t.Fail()
	}
}

func TestPipelined(t *testing.T) {
	expectedPipeIds := []pipeline.Pipe{1, 2, 3}

	builder := modules.NewBuilder(0, nil)
	builder.EnablePipelining(expectedPipeIds)
	builder.AddPiped(NewAdder)
	builder.AddPiped(NewMultiplier)

	core := builder.Build()
	if core.PipeCount() != len(expectedPipeIds) {
		t.Fail()
	}

	type AdderMultTestCase struct {
		A      int
		B      int
		Result int
	}

	testCasesMult := map[pipeline.Pipe]AdderMultTestCase{
		1: {A: 2, B: 3, Result: 6},
		2: {A: 2, B: 5, Result: 10},
		3: {A: 2, B: 6, Result: 12},
	}

	pipeIds := core.Pipes()
	for _, id := range pipeIds {
		var multer Multiplier
		core.MatchForPipe(id, &multer)
		tc := testCasesMult[id]
		actualResult := multer.Mult(tc.A, tc.B)
		if tc.Result != actualResult {
			t.Fail()
		}

		// The last result stored in the adder is the same as the result of multiplier,
		// since the multiple addings will add up to the multiplication answer.
		var adder Adder
		core.MatchForPipe(id, &adder)
		if adder.LastResult() != tc.Result || adder.LastResult() != actualResult {
			t.Fail()
		}
	}
}
