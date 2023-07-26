package modules

import (
	"fmt"
	"reflect"
	"sync"
)

var (
	registryMut sync.Mutex
	byInterface = make(map[reflect.Type]map[string]any)
	byName      = make(map[string]any)
)

// RegisterModule registers a constructor for a module implementation with the specified name.
// For example:
//
//	RegisterModule("chainedhotstuff", func() consensus.Rules { return chainedhotstuff.New() })
func RegisterModule[T any](name string, constructor func() T) {
	var t T
	moduleType := reflect.TypeOf(&t).Elem()

	registryMut.Lock()
	defer registryMut.Unlock()

	if _, ok := byName[name]; ok {
		panic(fmt.Sprintf("a module with name %s already exists", name))
	}
	byName[name] = constructor

	moduleRegistry, ok := byInterface[moduleType]
	if !ok {
		moduleRegistry = make(map[string]any)
		byInterface[moduleType] = moduleRegistry
	}

	moduleRegistry[name] = constructor
}

// GetModule constructs a new instance of the module with the specified name.
// GetModule returns true if the module is found, false otherwise.
// For example:
//
//	rules, ok := GetModule[consensus.Rules]("chainedhotstuff")
func GetModule[T any](name string) (out T, ok bool) {
	targetType := reflect.TypeOf(&out).Elem()

	registryMut.Lock()
	defer registryMut.Unlock()

	modules, ok := byInterface[targetType]
	if !ok {
		return out, false
	}

	ctor, ok := modules[name]
	if !ok {
		return out, false
	}

	return ctor.(func() T)(), true
}

// GetModuleUntyped returns a new instance of the named module, if it exists.
func GetModuleUntyped(name string) (m any, ok bool) {
	registryMut.Lock()
	defer registryMut.Unlock()

	ctor, ok := byName[name]
	if !ok {
		return nil, false
	}

	reflect.ValueOf(&m).Elem().Set(reflect.ValueOf(ctor).Call([]reflect.Value{})[0])

	return m, ok
}

// ListModules returns a map of interface names to module names.
func ListModules() map[string][]string {
	modules := make(map[string][]string)

	registryMut.Lock()
	defer registryMut.Unlock()

	for t, m := range byInterface {
		names := make([]string, 0, len(m))
		for name := range m {
			names = append(names, name)
		}
		modules[fmt.Sprintf("(%s).%s", t.PkgPath(), t.Name())] = names
	}

	return modules
}
