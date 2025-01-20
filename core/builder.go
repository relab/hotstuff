package core

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
)

// Builder is a helper for setting up client components.
type Builder struct {
	core       Core
	modules    []Component
	components ComponentList

	requireMandatoryComps bool
	opts                  *Options
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

func (b *Builder) RequireMandatoryComps() {
	b.requireMandatoryComps = true
}

// Options returns the options component.
func (b *Builder) Options() *Options {
	return b.opts
}

// Add adds components to the builder.
func (b *Builder) Add(modules ...any) {
	if !b.requireMandatoryComps {
		b.core.modules = append(b.core.modules, modules...)
		for _, component := range modules {
			if m, ok := component.(Component); ok {
				b.modules = append(b.modules, m)
			}
		}
		return
	}

	for _, component := range modules {
		switch v := component.(type) {
		case BlockChain:
			b.components.BlockChain = v
		case CommandCache:
			b.components.CommandCache = v
		case Configuration:
			b.components.Configuration = v
		case Consensus:
			b.components.Consensus = v
		case Crypto:
			b.components.Crypto = v
		case ExecutorExt:
			b.components.Executor = v
		case *EventLoop:
			b.components.EventLoop = v
		case ForkHandlerExt:
			b.components.ForkHandler = v
		case logging.Logger:
			b.components.Logger = v
		case Synchronizer:
			b.components.Synchronizer = v
		case VotingMachine:
			b.components.VotingMachine = v
		default:
			b.core.modules = append(b.core.modules, component)
			if m, ok := component.(Component); ok {
				b.modules = append(b.modules, m)
			}
		}
	}
}

// Build initializes all added components and returns the Core object.
func (b *Builder) Build() *Core {
	// reverse the order of the added components so that TryGet will find the latest first.
	for i, j := 0, len(b.core.modules)-1; i < j; i, j = i+1, j-1 {
		b.core.modules[i], b.core.modules[j] = b.core.modules[j], b.core.modules[i]
	}

	if b.requireMandatoryComps {
		b.components.Options = b.opts
		// Copy over the component pointers to core.
		b.core.components = b.components

		err := b.core.components.init(&b.core)
		if err != nil {
			panic(fmt.Sprintf("builder error: %v", err))
		}
	} else {
		b.Add(b.opts)
	}

	for _, component := range b.modules {
		component.InitComponent(&b.core)
	}
	return &b.core
}
