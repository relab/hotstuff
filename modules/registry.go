package modules

import (
	"fmt"
	"reflect"
	"sync"
)

var (
	registryMut sync.Mutex
	byInterface = make(map[reflect.Type]map[string]interface{})
	byName      = make(map[string]interface{})
)

// RegisterModule registers a module implementation with the specified name.
// constructor must be a function returning the interface of the module.
// For example:
//  RegisterModule("chainedhotstuff", func() consensus.Rules { return chainedhotstuff.New() })
func RegisterModule(name string, constructor interface{}) {
	ctorType := reflect.TypeOf(constructor)

	if ctorType.Kind() != reflect.Func && ctorType.NumOut() != 1 && ctorType.Out(0).Kind() != reflect.Interface {
		panic("invalid argument: constructor must be a function returning an interface")
	}

	ifaceType := ctorType.Out(0)

	registryMut.Lock()
	defer registryMut.Unlock()

	if _, ok := byName[name]; ok {
		panic(fmt.Sprintf("a module with name %s already exists", name))
	}
	byName[name] = constructor

	moduleRegistry, ok := byInterface[ifaceType]
	if !ok {
		moduleRegistry = make(map[string]interface{})
		byInterface[ifaceType] = moduleRegistry
	}

	moduleRegistry[name] = constructor
}

// GetModule retrieves a new instance of the module with the specified name.
// out must be a non-nil pointer to a variable with the interface type of the module.
// GetModule returns true if the module is found, false otherwise.
// For example:
//  var rules consensus.Rules
//  GetModule("chainedhotstuff", &rules)
func GetModule(name string, out interface{}) bool {
	outType := reflect.TypeOf(out)

	if outType.Kind() != reflect.Ptr {
		panic("invalid argument: out must be a non-nil pointer to an interface variable")
	}

	targetType := outType.Elem()

	registryMut.Lock()
	defer registryMut.Unlock()

	modules, ok := byInterface[targetType]
	if !ok {
		return false
	}

	ctor, ok := modules[name]
	if !ok {
		return false
	}

	reflect.ValueOf(out).Elem().Set(reflect.ValueOf(ctor).Call([]reflect.Value{})[0])
	return true
}

// GetModuleUntyped returns a new instance of the named module, if it exists.
func GetModuleUntyped(name string) (v interface{}, ok bool) {
	registryMut.Lock()
	defer registryMut.Unlock()

	ctor, ok := byName[name]
	if !ok {
		return nil, false
	}

	reflect.ValueOf(&v).Elem().Set(reflect.ValueOf(ctor).Call([]reflect.Value{})[0])

	return v, ok
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
