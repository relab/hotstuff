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
// To be able to access other modules from a struct, you will need to implement the Module interface from this package.
// The InitModule method of the Module interface gives your struct a pointer to the Modules object, which can be used
// to obtain pointers to the other modules. If your module will be interacting with the event loop, then this is the
// preferred location to set up observers or handlers for events.
//
// To give other modules access to the component, you will need to add a field to the Modules object, add a getter method
// on the Modules object, and add a check for you module's interface or type in the Builder's Register method.
// In general you should create an interface for your module if it is possible that someone might want to write their
// own version of it in the future.
//
// Finally, to set up the module system and its modules, you must create a Builder using the NewBuilder function,
// and then register all of the modules with the builder using the Register method. For example:
//
//  builder := NewBuilder()
//  // replace the logger
//  builder.Register(logging.New("foo"))
//  mods := builder.Build()
//
// If two modules satisfy the same interface, then the one that was registered last will be returned by the module system,
// though note that both modules will be initialized if they implement the Module interface.
package modules

import (
	"reflect"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
)

// Module is an interface for modules that need access to a client.
type Module interface {
	// InitModule gives the module access to the other modules.
	InitModule(mods *Modules)
}

// Modules is the base of the module system.
// It contains only a few core modules that are shared between replicas and clients.
type Modules struct {
	id            hotstuff.ID
	logger        logging.Logger
	metricsLogger MetricsLogger
	eventLoop     *eventloop.EventLoop
	modulesByType map[reflect.Type]interface{}
}

// ID returns the id of this client.
func (mods Modules) ID() hotstuff.ID {
	return mods.id
}

// Logger returns the logger.
func (mods Modules) Logger() logging.Logger {
	return mods.logger
}

// MetricsLogger returns the metrics logger.
func (mods Modules) MetricsLogger() MetricsLogger {
	if mods.metricsLogger == nil {
		return NopLogger()
	}
	return mods.metricsLogger
}

// EventLoop returns the event loop.
func (mods Modules) EventLoop() *eventloop.EventLoop {
	return mods.eventLoop
}

// MetricsEventLoop returns the metrics event loop.
// The metrics event loop is used for processing of measurement data.
//
// Deprecated: The metrics event loop is no longer separate from the main event loop. Use EventLoop() instead.
func (mods Modules) MetricsEventLoop() *eventloop.EventLoop {
	return mods.EventLoop()
}

// GetModuleByType makes it possible to get a module based on its real type.
// This is useful for getting modules that do not implement any known module interface.
// The method returns true if a module was found, false otherwise.
//
// NOTE: dest MUST be a pointer to a variable of the desired type.
// For example:
//  var module MyModule
//  if mods.GetModuleByType(&module) { ... }
func (mods Modules) GetModuleByType(dest interface{}) bool {
	outType := reflect.TypeOf(dest)
	if outType.Kind() != reflect.Ptr {
		panic("invalid argument: out must be a non-nil pointer to an interface variable")
	}
	targetType := outType.Elem()
	if m, ok := mods.modulesByType[targetType]; ok {
		reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(m))
		return true
	}
	return false
}

// Builder is a helper for setting up client modules.
type Builder struct {
	mods    Modules
	modules []Module
}

// NewBuilder returns a new builder.
func NewBuilder(id hotstuff.ID) Builder {
	bl := Builder{mods: Modules{
		id:            id,
		logger:        logging.New(""),
		eventLoop:     eventloop.New(1000),
		modulesByType: make(map[reflect.Type]interface{}),
	}}
	return bl
}

// Register registers the modules with the builder.
func (b *Builder) Register(modules ...interface{}) {
	for _, module := range modules {
		if m, ok := module.(logging.Logger); ok {
			b.mods.logger = m
		}
		if m, ok := module.(MetricsLogger); ok {
			b.mods.metricsLogger = m
		}
		if m, ok := module.(Module); ok {
			b.modules = append(b.modules, m)
		}
		b.mods.modulesByType[reflect.TypeOf(module)] = module
	}
}

// Build initializes all registered modules and returns the Modules object.
func (b *Builder) Build() *Modules {
	for _, module := range b.modules {
		module.InitModule(&b.mods)
	}
	return &b.mods
}
