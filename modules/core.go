// Package modules contains the module system used in the hotstuff project.
// The module system allows us to use different implementations of key components,
// such as the crypto module or the consensus module,
// and ensures that each module has access to the other modules it depends on.
//
// There are two main reason one might want to use the module system for a component:
//
// 1. To give the component access to other modules.
//
// 2. To give other modules access to the component.
//
// To be able to access other modules from a struct, you will need to implement the Module interface from this package.
// The InitModule method of the Module interface gives your struct a pointer to the Core object, which can be used
// to obtain pointers to the other modules.
// If your module will be interacting with the event loop,
// then this method is the preferred location to set up handlers for events.
//
// Finally, to set up the module system and its modules, you must create a Builder using the NewBuilder function,
// and then all your modules to the builder using the Add method. For example:
//
//	builder := NewBuilder()
//	// replace the logger
//	builder.Add(logging.New("foo"))
//	mods := builder.Build()
//
// If two modules satisfy the same interface, then the one that was registered last will be returned by the module system,
// though note that both modules will be initialized if they implement the Module interface.
//
// After building the module system, you can use the TryGet or Get methods to get pointers to the modules:
//
//	var module MyModule
//	mods.Get(&module)
package modules

import (
	"fmt"
	"reflect"

	"github.com/relab/hotstuff"
)

// Module is an interface for initializing modules.
type Module interface {
	InitModule(mods *Core)
}

// Provider is an interface for specifying the type that a module provides.
// In order to be available to other modules through the Get methods,
// a module must provide an implementation of this interface.
// Most modules can simply embed the Implements struct to become a Provider.
type Provider interface {
	ModuleType() any
}

type providedAs struct {
	Provider
	module any
}

func (p providedAs) InitModule(mods *Core) {
	if m, ok := p.module.(Module); ok {
		m.InitModule(mods)
	}
}

func (p providedAs) Unwrap() any {
	return p.module
}

// As makes the module a provider of type T.
// This can for example be used with modules that cannot implement Provider themselves,
// or modules that implement multiple interfaces.
func As[T any](module any) Provider {
	var zero T
	tt := reflect.TypeOf(&zero).Elem()
	if !reflect.TypeOf(module).AssignableTo(tt) {
		panic(fmt.Sprintf("%T is not compatible with %s", module, tt))
	}

	return providedAs{
		Provider: Implements[T]{},
		module:   module,
	}
}

// Implements is a Provider for type T.
// By embedding this struct, a module can become a provider for T.
//
// For example:
//
//	type MyModule interface { ... }
//
//	type MyModuleImpl struct {
//		modules.Implements[MyModule]
//	}
type Implements[T any] struct{}

func (p Implements[T]) ModuleType() any {
	return new(T)
}

// Core is the base of the module system.
// It contains only a few core modules that are shared between replicas and clients.
type Core struct {
	modulesByType map[reflect.Type]any
}

// TryGet attempts to find a module for ptr.
// TryGet returns true if a module was stored in ptr, false otherwise.
//
// NOTE: ptr must be a non-nil pointer to a type that has been provided to the module system.
//
// Example:
//
//	builder := modules.New()
//	builder.Add(MyModuleImpl{})
//	mods = builder.Build()
//
//	var module MyModule
//	if mods.TryGet(&module) {
//		// success
//	}
func (mods Core) TryGet(ptr any) bool {
	v := reflect.ValueOf(ptr)
	if !v.IsValid() {
		panic("nil value given")
	}
	pt := v.Type()
	if pt.Kind() != reflect.Ptr {
		panic("only pointer values allowed")
	}

	if m, ok := mods.modulesByType[pt.Elem()]; ok {
		v.Elem().Set(reflect.ValueOf(m))
		return true
	}
	return false
}

// Get finds compatible modules for the given pointers.
//
// NOTE: pointers must only contain non-nil pointers to types that have been provided to the module system.
// Get panics if one of the given arguments is not a pointer, or if a compatible module is not found.
//
// Example:
//
//	builder := modules.New()
//	builder.Add(MyModuleImpl{})
//	mods = builder.Build()
//
//	var module MyModule
//	mods.Get(&module)
func (mods *Core) Get(pointers ...any) {
	if len(pointers) == 0 {
		panic("no pointers given")
	}
	for _, ptr := range pointers {
		if !mods.TryGet(ptr) {
			panic(fmt.Sprintf("module of type %s not found", reflect.TypeOf(ptr).Elem()))
		}
	}
}

// Builder is a helper for setting up client modules.
type Builder struct {
	mods    Core
	modules []Module
	opts    *Options
}

// NewBuilder returns a new builder.
func NewBuilder(id hotstuff.ID, pk hotstuff.PrivateKey) Builder {
	bl := Builder{
		mods: Core{
			modulesByType: make(map[reflect.Type]any),
		},
		opts: &Options{
			id:                 id,
			privateKey:         pk,
			connectionMetadata: make(map[string]string),
		},
	}
	return bl
}

// Options returns the options module.
func (b *Builder) Options() *Options {
	return b.opts
}

// Provide registers module as a module provider.
//
// Example:
//
//	builder := modules.NewBuilder()
//	builder.Provide(MyModuleImpl{}, new(MyModule))
func (b *Builder) Provide(module any, types ...any) {
	if len(types) == 0 {
		b.mods.modulesByType[reflect.TypeOf(module)] = module
		return
	}

	t := reflect.TypeOf(module)

	for _, ptr := range types {
		pt := reflect.TypeOf(ptr)
		if pt == nil {
			panic("nil value")
		}
		if pt.Kind() != reflect.Pointer {
			panic("only pointer values allowed in 'types'")
		}

		if !t.AssignableTo(pt.Elem()) {
			panic(fmt.Sprintf("module of type %s is not assignable to type %s", t, pt))
		}

		b.mods.modulesByType[pt.Elem()] = module
	}
}

// Add adds modules to the builder.
func (b *Builder) Add(modules ...any) {
	for _, module := range modules {
		if p, ok := module.(Provider); ok {
			if pa, ok := p.(providedAs); ok {
				b.Provide(pa.Unwrap(), p.ModuleType())
			} else {
				b.Provide(module, p.ModuleType())
			}
		}
		if m, ok := module.(Module); ok {
			b.modules = append(b.modules, m)
		}
	}
}

// Build initializes all added modules and returns the Core object.
func (b *Builder) Build() *Core {
	b.Add(b.opts)
	for _, module := range b.modules {
		module.InitModule(&b.mods)
	}
	return &b.mods
}
