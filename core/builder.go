package core

import "github.com/relab/hotstuff"

// Builder is a helper for setting up client module.
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

// Add adds module to the builder.
func (b *Builder) Add(module ...any) {
	b.core.modules = append(b.core.modules, module...)
	for _, mod := range module {
		if m, ok := mod.(Module); ok {
			b.modules = append(b.modules, m)
		}
	}
}

// Build initializes all added module and returns the Core object.
func (b *Builder) Build() *Core {
	// reverse the order of the added module so that TryGet will find the latest first.
	for i, j := 0, len(b.core.modules)-1; i < j; i, j = i+1, j-1 {
		b.core.modules[i], b.core.modules[j] = b.core.modules[j], b.core.modules[i]
	}
	// add the Options last so that it can be overridden by user.
	b.Add(b.opts)
	for _, mod := range b.modules {
		mod.InitModule(&b.core)
	}
	return &b.core
}
