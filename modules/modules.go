// Package modules contains the base of the module system used in the hotstuff project.
// The module system allows us to use different implementations of key components,
// such as the crypto module or the consensus module, and ensures that a module has
// access to the other modules it depends on.
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
	dataLogger    DataLogger
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

// DataLogger returns the data logger.
func (mod Modules) DataLogger() DataLogger {
	if mod.dataLogger == nil {
		return NopLogger()
	}
	return mod.dataLogger
}

// DataEventLoop returns the data event loop.
// The data event loop is used for processing of measurement data.
func (mod Modules) DataEventLoop() *eventloop.EventLoop {
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
		if m, ok := module.(DataLogger); ok {
			b.mod.dataLogger = m
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
