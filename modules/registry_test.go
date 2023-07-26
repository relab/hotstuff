package modules_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/relab/hotstuff/modules"
	"golang.org/x/exp/slices"
)

func init() {
	// register some modules
	modules.RegisterModule("frobulator", func() moduleIface {
		return module{}
	})

	modules.RegisterModule("fizzbuzz", func() moduleIface {
		return fizzbuzz{}
	})

	modules.RegisterModule("fizzer", func() fizzer {
		return fizzbuzz{}
	})

	modules.RegisterModule("buzzer", func() buzzer {
		return fizzbuzz{}
	})
}

func TestModuleRegistry(t *testing.T) {
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

func TestListModules(t *testing.T) {
	getName := func(t reflect.Type) string {
		return fmt.Sprintf("(%s).%s", t.PkgPath(), t.Name())
	}

	want := map[string][]string{
		getName(reflect.TypeOf((*moduleIface)(nil)).Elem()): {"frobulator", "fizzbuzz"},
		getName(reflect.TypeOf((*fizzer)(nil)).Elem()):      {"fizzer"},
		getName(reflect.TypeOf((*buzzer)(nil)).Elem()):      {"buzzer"},
	}

	list := modules.ListModules()

	for iface, names := range want {
		if got, ok := list[iface]; !ok {
			t.Errorf("expected interface '%s' to be in list of modules", iface)
		} else {
			for _, name := range names {
				if !slices.Contains(got, name) {
					t.Errorf("expected '%s' to be in list of '%s' implementations", name, iface)
				}
			}
		}
	}
}

type moduleIface interface {
	frobulate(i *int)
}

type module struct{}

func (module) frobulate(i *int) {
	*i++
}

type fizzer interface {
	fizz(i int)
}

type buzzer interface {
	buzz(i int)
}

type fizzbuzz struct{}

func (f fizzbuzz) fizz(_ int) { panic("not implemented") }

func (f fizzbuzz) buzz(_ int) { panic("not implemented") }

func (f fizzbuzz) frobulate(_ *int) { panic("not implemented") }
