package core

import "github.com/relab/hotstuff"

// Builder is a helper for setting up client components.
type Builder struct {
	core       Core
	components []Component
	opts       *Options
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

// Options returns the options component.
func (b *Builder) Options() *Options {
	return b.opts
}

// Add adds components to the builder.
func (b *Builder) Add(components ...any) {
	b.core.modules = append(b.core.modules, components...)
	for _, component := range components {
		if m, ok := component.(Component); ok {
			b.components = append(b.components, m)
		}
	}
}

// Build initializes all added components and returns the Core object.
func (b *Builder) Build() *Core {
	// reverse the order of the added components so that TryGet will find the latest first.
	for i, j := 0, len(b.core.modules)-1; i < j; i, j = i+1, j-1 {
		b.core.modules[i], b.core.modules[j] = b.core.modules[j], b.core.modules[i]
	}
	// add the Options last so that it can be overridden by user.
	b.Add(b.opts)
	for _, component := range b.components {
		component.InitComponent(&b.core)
	}
	return &b.core
}
