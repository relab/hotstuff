package modules_test

import (
	"testing"

	"github.com/relab/hotstuff/modules"
)

type Counter interface {
	Increment(name string)
	Count(name string) int
}

type counterImpl struct {
	counters map[string]int
}

func (c counterImpl) Increment(name string) { c.counters[name]++ }
func (c counterImpl) Count(name string) int { return c.counters[name] }

func NewCounter() *counterImpl { //nolint:revive
	return &counterImpl{
		counters: make(map[string]int),
	}
}

type Greeter interface {
	Greet(name string) string
}

type greeterImpl struct {
	// declares dependencies on other modules
	counter Counter
}

func (g greeterImpl) Greet(name string) string {
	g.counter.Increment(name)
	return "Hello, " + name
}

func NewGreeter() *greeterImpl { //nolint:revive
	return &greeterImpl{}
}

func (g *greeterImpl) InitModule(mods *modules.Core) {
	mods.Get(&g.counter)
}

func TestModule(t *testing.T) {
	builder := modules.NewBuilder(0, nil)
	builder.Add(NewCounter(), NewGreeter())

	mods := builder.Build()

	var (
		counter Counter
		greeter Greeter
	)

	mods.Get(&counter, &greeter)

	if greeter.Greet("John") != "Hello, John" {
		t.Fail()
	}

	if counter.Count("John") != 1 {
		t.Fail()
	}
}
