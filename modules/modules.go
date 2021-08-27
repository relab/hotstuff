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
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/logging"
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
	dataEventLoop *eventloop.EventLoop
}

// ID returns the id of this client.
func (mod Modules) ID() hotstuff.ID {
	return mod.id
}

// Logger returns the logger.
func (mod Modules) Logger() logging.Logger {
	return mod.logger
}

// MetricsLogger returns the metrics logger.
func (mod Modules) MetricsLogger() MetricsLogger {
	if mod.metricsLogger == nil {
		return NopLogger()
	}
	return mod.metricsLogger
}

// MetricsEventLoop returns the metrics event loop.
// The metrics event loop is used for processing of measurement data.
func (mod Modules) MetricsEventLoop() *eventloop.EventLoop {
	return mod.dataEventLoop
}

// Builder is a helper for setting up client modules.
type Builder struct {
	mod     Modules
	modules []Module
}

// NewBuilder returns a new builder.
func NewBuilder(id hotstuff.ID) Builder {
	bl := Builder{mod: Modules{
		id:            id,
		logger:        logging.New(""),
		dataEventLoop: eventloop.New(100),
	}}
	return bl
}

// Register registers the modules with the builder.
func (b *Builder) Register(modules ...interface{}) {
	for _, module := range modules {
		if m, ok := module.(logging.Logger); ok {
			b.mod.logger = m
		}
		if m, ok := module.(MetricsLogger); ok {
			b.mod.metricsLogger = m
		}
		if m, ok := module.(Module); ok {
			b.modules = append(b.modules, m)
		}
	}
}

// Build initializes all registered modules and returns the Modules object.
func (b *Builder) Build() *Modules {
	for _, module := range b.modules {
		module.InitModule(&b.mod)
	}
	return &b.mod
}
