package modules_test

import (
	"testing"

	"github.com/relab/hotstuff"
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

func (m *multiplierImpl) InitModule(mods *modules.Core, _ modules.ScopeInfo) {
	mods.GetScoped(m, &m.adder) // Requires an adder from the same pipe
}

func TestPipeliningDisabled(t *testing.T) {
	builder := modules.NewBuilder(0, nil)

	adders := builder.CreateScope(NewAdder)
	multers := builder.CreateScope(NewMultiplier)

	builder.AddScoped(adders, multers)

	mods := builder.Build()
	if mods.ScopeCount() > 0 {
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
	pipes := 3

	builder := modules.NewBuilder(0, nil)
	builder.EnablePipelining(pipes)

	adders := builder.CreateScope(NewAdder)
	multers := builder.CreateScope(NewMultiplier)

	builder.AddScoped(adders, multers)

	core := builder.Build()
	if core.ScopeCount() != pipes {
		t.Fail()
	}

	type AdderMultTestCase struct {
		A      int
		B      int
		Result int
	}

	testCasesMult := map[hotstuff.Pipe]AdderMultTestCase{
		1: {A: 2, B: 3, Result: 6},
		2: {A: 2, B: 5, Result: 10},
		3: {A: 2, B: 6, Result: 12},
	}

	scopeIds := core.Scopes()
	for _, id := range scopeIds {
		var multer Multiplier
		core.MatchForScope(id, &multer)
		tc := testCasesMult[id]
		actualResult := multer.Mult(tc.A, tc.B)
		if tc.Result != actualResult {
			t.Fail()
		}

		// The last result stored in the adder is the same as the result of multiplier,
		// since the multiple addings will add up to the multiplication answer.
		var adder Adder
		core.MatchForScope(id, &adder)
		if adder.LastResult() != tc.Result || adder.LastResult() != actualResult {
			t.Fail()
		}
	}
}
