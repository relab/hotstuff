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
// Finally, to set up the module system and its modules, you must create a CoreBuilder using the NewCoreBuilder function,
// and then register all of the modules with the builder using the Register method. For example:
//
//	builder := NewCoreBuilder()
//	// replace the logger
//	builder.Register(logging.New("foo"))
//	mods := builder.Build()
//
// If two modules satisfy the same interface, then the one that was registered last will be returned by the module system,
// though note that both modules will be initialized if they implement the CoreModule interface.
package modules

import (
	"fmt"
	"reflect"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
)

// CoreModule is an interface for modules that need access to a client.
type CoreModule interface {
	// InitModule gives the module access to the other modules.
	InitModule(mods *Core)
}

type Provider interface {
	ProvideModule() (moduleType any)
}

type Implements[T any] struct {
}

func (p Implements[T]) ProvideModule() (moduleType any) {
	return new(T)
}

func Provide[T any]() Provider {
	return Implements[T]{}
}

// Core is the base of the module system.
// It contains only a few core modules that are shared between replicas and clients.
type Core struct {
	id hotstuff.ID

	logger        logging.Logger
	metricsLogger MetricsLogger
	eventLoop     *eventloop.EventLoop

	modulesByType map[reflect.Type]any
}

// ID returns the id of this client.
func (mods Core) ID() hotstuff.ID {
	return mods.id
}

// Logger returns the logger.
func (mods Core) Logger() logging.Logger {
	return mods.logger
}

// MetricsLogger returns the metrics logger.
func (mods Core) MetricsLogger() MetricsLogger {
	if mods.metricsLogger == nil {
		return NopLogger()
	}
	return mods.metricsLogger
}

// EventLoop returns the event loop.
func (mods Core) EventLoop() *eventloop.EventLoop {
	return mods.eventLoop
}

// MetricsEventLoop returns the metrics event loop.
// The metrics event loop is used for processing of measurement data.
//
// Deprecated: The metrics event loop is no longer separate from the main event loop. Use EventLoop() instead.
func (mods Core) MetricsEventLoop() *eventloop.EventLoop {
	return mods.EventLoop()
}

// TryGet attempts to find a module for ptr.
// TryGet returns true if a module was stored in ptr, false otherwise.
//
// NOTE: ptr must be a pointer to a type that has been provided to the module system.
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

	if m, ok := mods.modulesByType[pt.Elem()]; ok {
		v.Elem().Set(reflect.ValueOf(m))
		return true
	}
	return false
}

// Get finds a module for ptr.
//
// NOTE: ptr must be a pointer to a type that has been provided to the module system.
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

// CoreBuilder is a helper for setting up client modules.
type CoreBuilder struct {
	mods    Core
	modules []CoreModule
}

func NewBuilder() CoreBuilder {
	return NewCoreBuilder(0)
}

// NewCoreBuilder returns a new builder.
func NewCoreBuilder(id hotstuff.ID) CoreBuilder {
	bl := CoreBuilder{mods: Core{
		id:            id,
		logger:        logging.New(""),
		eventLoop:     eventloop.New(1000),
		modulesByType: make(map[reflect.Type]any),
	}}
	return bl
}

// Provide registers module as a module provider.
//
// Example:
//
//	builder := modules.NewBuilder()
//	builder.Provide(MyModuleImpl{}, new(MyModule))
func (b *CoreBuilder) Provide(module any, types ...any) {
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
func (b *CoreBuilder) Add(modules ...any) {
	for _, module := range modules {
		if m, ok := module.(logging.Logger); ok {
			b.mods.logger = m
		}
		if m, ok := module.(MetricsLogger); ok {
			b.mods.metricsLogger = m
		}
		if m, ok := module.(CoreModule); ok {
			b.modules = append(b.modules, m)
		}
		if p, ok := module.(Provider); ok {
			pt := p.ProvideModule()
			b.Provide(module, pt)
		}
		// b.mods.modulesByType[reflect.TypeOf(module)] = module
	}
}

// Build initializes all added modules and returns the Core object.
func (b *CoreBuilder) Build() *Core {
	for _, module := range b.modules {
		module.InitModule(&b.mods)
	}
	return &b.mods
}
