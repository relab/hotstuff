// Package modules contains the base of the module system used in the hotstuff project.
// The module system allows us to use different implementations of key components,
// such as the crypto module or the consensus module, and ensures that a module has
// access to the other modules it depends on.
package modules

import (
	"context"

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
	eventloop     *eventloop.EventLoop
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

// EventLoop returns the metrics event loop.
// The metrics event loop is used for processing of measurement data.
func (mods Modules) EventLoop() *eventloop.EventLoop {
	return mods.eventloop
}

// Run starts the event loop and returns when the event loop exits.
func (mods *Modules) Run(ctx context.Context) {
	mainDone := make(chan struct{})
	go func() {
		mods.EventLoop().Run(ctx)
		close(mainDone)
	}()

	<-mainDone
}

// Builder is a helper for setting up client modules.
type Builder struct {
	mods    Modules
	modules []Module
}

// NewBuilder returns a new builder.
func NewBuilder(id hotstuff.ID) Builder {
	bl := Builder{mods: Modules{
		id:        id,
		logger:    logging.New(""),
		eventloop: eventloop.New(100),
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
		if m, ok := module.(*eventloop.EventLoop); ok {
			b.mods.eventloop = m
		}
		if m, ok := module.(Module); ok {
			b.modules = append(b.modules, m)
		}
	}
}

// Build initializes all registered modules and returns the Modules object.
func (b *Builder) Build() *Modules {
	for _, module := range b.modules {
		module.InitModule(&b.mods)
	}
	return &b.mods
}
