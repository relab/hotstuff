// Package components contains the component system used in the hotstuff project.
// The component system allows us to use different implementations of key components,
// such as the crypto component or the consensus component,
// and ensures that each component has access to the other components it depends on.
//
// There are two main reason one might want to use the component system for a component:
//
// 1. To give the component access to other components.
//
// 2. To give other components access to the component.
//
// To be able to access other components from a struct, you will need to implement the component interface from this package.
// The InitComponent method of the component interface gives your struct a pointer to the Core object, which can be used
// to obtain pointers to the other components.
// If your component will be interacting with the event loop,
// then this method is the preferred location to set up handlers for events.
//
// Finally, to set up the component system and its components, you must create a Builder using the NewBuilder function,
// and then all your components to the builder using the Add method. For example:
//
//	builder := NewBuilder()
//	// replace the logger
//	builder.Add(logging.New("foo"))
//	mods := builder.Build()
//
// If two components satisfy the same interface, then the one that was registered last will be returned by the component system,
// though note that both components will be initialized if they implement the component interface.
//
// After building the component system, you can use the TryGet or Get methods to get pointers to the components:
//
//	var component Mycomponent
//	mods.Get(&component)
package core

import (
	"fmt"
	"reflect"

	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

// Component is an interface for initializing components.
type Component interface {
	InitComponent(mods *Core)
}

type ComponentList struct {
	BlockChain    BlockChain
	CommandCache  CommandCache
	Configuration Configuration
	Consensus     Consensus
	Crypto        Crypto
	EventLoop     *EventLoop
	Executor      ExecutorExt
	ForkHandler   ForkHandlerExt
	Logger        logging.Logger
	Synchronizer  Synchronizer
	VotingMachine VotingMachine
	Options       *Options
}

func (c *ComponentList) init(core *Core) error {
	v := reflect.ValueOf(*c)
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		iface := field.Interface()
		if iface == nil {
			return fmt.Errorf("component %s cannot be nil", field.Type().Name())
		}
		if comp, ok := iface.(Component); ok {
			comp.InitComponent(core)
		}
	}
	return nil
}

// Core is the base of the component system.
// It contains only a few core components that are shared between replicas and clients.
type Core struct {
	modules    []any
	components ComponentList

	// TODO: This is a module, make a new system to acquire this.
	leaderRotation modules.LeaderRotation
}

// TryGet attempts to find a component for ptr.
// TryGet returns true if a component was stored in ptr, false otherwise.
//
// NOTE: ptr must be a non-nil pointer to a type that has been provided to the component system.
//
// Example:
//
//	builder := core.NewBuilder()
//	builder.Add(MycomponentImpl{})
//	mods = builder.Build()
//
//	var component Mycomponent
//	if mods.TryGet(&component) {
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

// Get finds compatible components for the given pointers.
//
// NOTE: pointers must only contain non-nil pointers to types that have been provided to the component system.
// Get panics if one of the given arguments is not a pointer, or if a compatible component is not found.
//
// Example:
//
//	builder := core.NewBuilder()
//	builder.Add(MycomponentImpl{})
//	mods = builder.Build()
//
//	var component Mycomponent
//	mods.Get(&component)
func (mods *Core) Get(pointers ...any) {
	if len(pointers) == 0 {
		panic("no pointers given")
	}
	for _, ptr := range pointers {
		if !mods.TryGet(ptr) {
			panic(fmt.Sprintf("component of type %s not found", reflect.TypeOf(ptr).Elem()))
		}
	}
}

// Components returns a copied struct containing
// pointers (and interface refs) to the mandatory
// components of Core.
func (mods *Core) Components() ComponentList {
	return mods.components
}
