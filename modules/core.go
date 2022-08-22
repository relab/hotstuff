// Package modules contains the base of the module system used in the hotstuff project.
// The module system allows us to use different implementations of key components,
// such as the crypto module or the consensus module, and ensures that a module has
// access to the other modules it depends on.
//
// This package defines a minimal set of modules that are common to both replicas and clients.
// The consensus package extends this set of modules with many more modules that make up the consensus protocol.
// If your module does not need access to any of the consensus modules, then you can use this package instead.
//
// There are two main reason one might want to use the module system for a component:
//
// 1. To give the component access to other modules.
//
// 2. To give other modules access to the component.
//
// To be able to access other modules from a struct, you will need to implement the CoreModule interface from this package.
// The InitModule method of the CoreModule interface gives your struct a pointer to the Core object, which can be used
// to obtain pointers to the other modules. If your module will be interacting with the event loop, then this is the
// preferred location to set up observers or handlers for events.
//
// To give other modules access to the component, you will need to add a field to the Core object, add a getter method
// on the Core object, and add a check for you module's interface or type in the CoreBuilder's Register method.
// In general you should create an interface for your module if it is possible that someone might want to write their
// own version of it in the future.
//
// Finally, to set up the module system and its modules, you must create a CoreBuilder using the NewBuilder function,
// and then register all of the modules with the builder using the Register method. For example:
//
//	builder := NewBuilder()
//	// replace the logger
//	builder.Add(logging.New("foo"))
//	mods := builder.Build()
//
// If two modules satisfy the same interface, then the one that was registered last will be returned by the module system,
// though note that both modules will be initialized if they implement the CoreModule interface.
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

// Core is the base of the module system.
// It contains only a few core modules that are shared between replicas and clients.
type Core struct {
	modules []any
}

// TryGet attempts to find a module for ptr.
// TryGet returns true if a module was stored in ptr, false otherwise.
//
// NOTE: ptr must be a non-nil pointer to a type that has been provided to the module system.
//
// Example:
//
//	builder := modules.New()
//	builder.Provide(MyModuleImpl{}, new(MyModule))
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

	for _, m := range mods.modules {
		mv := reflect.ValueOf(m)
		if mv.Type().AssignableTo(pt.Elem()) {
			v.Elem().Set(mv)
			return true
		}
	}

	return false
}

// Get finds a module for ptr.
//
// NOTE: ptr must be a non-nil pointer to a type that has been provided to the module system.
// Get panics if ptr is not a pointer, or if a compatible module is not found.
//
// Example:
//
//	builder := modules.New()
//	builder.Provide(MyModuleImpl{}, new(MyModule))
//	mods = builder.Build()
//
//	var module MyModule
//	mods.Get(&module)
func (mods *Core) Get(ptr any) {
	if !mods.TryGet(ptr) {
		panic(fmt.Sprintf("module of type %s not found", reflect.TypeOf(ptr).Elem()))
	}
}

// GetAll finds a module for all the given pointers.
//
// NOTE: pointers must only contain non-nil pointers to types that have been provided to the module system.
// GetAll panics if one of the given pointers is not a pointer, or if a compatible module is not found.
func (mods *Core) GetAll(pointers ...any) {
	for _, ptr := range pointers {
		mods.Get(ptr)
	}
}

// Builder is a helper for setting up client modules.
type Builder struct {
	core    Core
	modules []Module
	opts    *Options
}

// NewBuilder returns a new builder.
func NewBuilder(id hotstuff.ID, pk hotstuff.PrivateKey) Builder {
	bl := Builder{
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

// Add adds modules to the builder.
func (b *Builder) Add(modules ...any) {
	b.core.modules = append(b.core.modules, modules...)
	for _, module := range modules {
		if m, ok := module.(Module); ok {
			b.modules = append(b.modules, m)
		}
	}
}

// Build initializes all added modules and returns the Core object.
func (b *Builder) Build() *Core {
	// reverse the order of the added modules so that TryGet will find the latest first.
	for i, j := 0, len(b.core.modules)-1; i < j; i, j = i+1, j-1 {
		b.core.modules[i], b.core.modules[j] = b.core.modules[j], b.core.modules[i]
	}
	// add the Options last so that it can be overridden by user.
	b.Add(b.opts)
	for _, module := range b.modules {
		module.InitModule(&b.core)
	}
	return &b.core
}
