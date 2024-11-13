// Package modules contains the module system used in the hotstuff project.
// The module system allows us to use different implementations of key components,
// such as the crypto module or the consensus module,
// and ensures that each module has access to the other modules it depends on.
//
// There are two main reason one might want to use the module system for a component:
//
// 1. To give the component access to other modules.
//
// 2. To give other modules access to the component.
//
// To be able to access other modules from a struct, you will need to implement the Module interface from this package.
// The InitModule method of the Module interface gives your struct a pointer to the Core object, which can be used
// to obtain pointers to the other modules.
// If your module will be interacting with the event loop,
// then this method is the preferred location to set up handlers for events.
//
// Finally, to set up the module system and its modules, you must create a Builder using the NewBuilder function,
// and then all your modules to the builder using the Add method. For example:
//
//	builder := NewBuilder()
//	// replace the logger
//	builder.Add(logging.New("foo"))
//	mods := builder.Build()
//
// If two modules satisfy the same interface, then the one that was registered last will be returned by the module system,
// though note that both modules will be initialized if they implement the Module interface.
//
// After building the module system, you can use the TryGet or Get methods to get pointers to the modules:
//
//	var module MyModule
//	mods.Get(&module)
package modules

import (
	"fmt"
	"reflect"

	"github.com/relab/hotstuff"
)

type moduleScope []Module

type InitOptions struct {
	IsPipeliningEnabled     bool
	InstanceCount           int
	ModuleConsensusInstance hotstuff.Instance
}

// Module is an interface for initializing modules.
type Module interface {
	InitModule(mods *Core, buildOpt InitOptions)
}

// Core is the base of the module system.
// It contains only a few core modules that are shared between replicas and clients.
type Core struct {
	staticModules       []any
	scopedModules       map[hotstuff.Instance][]any
	isPipeliningEnabled bool
}

// TryGet attempts to find a module for ptr.
// TryGet returns true if a module was stored in ptr, false otherwise.
//
// NOTE: ptr must be a non-nil pointer to a type that has been provided to the module system.
//
// Example:
//
//	builder := modules.New()
//	builder.Add(MyModuleImpl{})
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

	for _, m := range mods.staticModules {
		mv := reflect.ValueOf(m)
		if mv.Type().AssignableTo(pt.Elem()) {
			v.Elem().Set(mv)
			return true
		}
	}

	return false
}

// Get finds compatible modules for the given pointers.
//
// NOTE: pointers must only contain non-nil pointers to types that have been provided to the module system.
// Get panics if one of the given arguments is not a pointer, or if a compatible module is not found.
//
// Example:
//
//	builder := modules.New()
//	builder.Add(MyModuleImpl{})
//	mods = builder.Build()
//
//	var module MyModule
//	mods.Get(&module)
func (mods *Core) Get(pointers ...any) {
	if len(pointers) == 0 {
		panic("no pointers given")
	}
	for _, ptr := range pointers {
		if !mods.TryGet(ptr) {
			panic(fmt.Sprintf("module of type %s not found", reflect.TypeOf(ptr).Elem()))
		}
	}
}

// GetScoped does the same as Get and additionally searches for pointers in the same scope as moduleInScope.
// If pipelining is not enabled, Get is called internally instead.
//
// NOTE: pointers must only contain non-nil pointers to types that have been provided to the module system
// as a scoped module.
// GetScoped panics if one of the given arguments is not a pointer, if a compatible module is not found,
// if the module was not in a scope or if the module was not in the same scope as moduleInScope.
//
// Example:
//
//	type OtherModule interface {
//		Foo()
//	}
//
//	type MyModuleImpl struct {
//		otherModule OtherModule
//	}
//
//	func (m *MyModuleImpl) InitModule(mods *modules.Core, buildOpt modules.InitOptions) {
//		mods.GetScoped(m, &m.otherModule) // Requires an OtherModule from the same pipe
//	}
//
//	func main() {
//		// TODO: Fix this doc
//		consensusInstanceCount := 3
//
//		builder := modules.NewBuilder(0, nil)
//		builder.EnablePipelining(consensusInstanceCount)
//		builder.AddScoped(NewMyModuleImpl)
//		builder.AddScoped(NewOtherModuleImpl)
//		builder.Build() // InitModule is called here
//	}
func (mods *Core) GetScoped(moduleInScope Module, pointers ...any) {
	if len(pointers) == 0 {
		panic("no pointers given")
	}

	if !mods.isPipeliningEnabled {
		mods.Get(pointers...)
		return
	}

	for _, ptr := range pointers {
		if !mods.TryGet(ptr) {
			if !mods.tryGetFromScope(moduleInScope, ptr) {
				panic(fmt.Sprintf("scoped module of type %s not found", reflect.TypeOf(ptr).Elem()))
			}
		}
	}
}

// tryGetFromScope attempts to find a module for ptr which also happens to be in the same
// instance as moduleInScope, false otherwise.
// tryGetFromScope returns true if a module was successflully stored in ptr, false otherwise.
// If pipelining was not enabled, TryGet is called implicitly.
func (mods *Core) tryGetFromScope(moduleInScope Module, ptr any) bool {
	if len(mods.scopedModules) == 0 {
		return mods.TryGet(ptr)
	}

	v := reflect.ValueOf(ptr)
	if !v.IsValid() {
		panic("ptr value cannot be nil")
	}
	pt := v.Type()
	if pt.Kind() != reflect.Ptr {
		panic("only pointer values allowed")
	}

	correctInstance := hotstuff.ZeroInstance
	for instance := range mods.scopedModules {
		scope := mods.scopedModules[instance]
		// Check if self is in scope
		for _, module := range scope {
			// TODO: Verify if equality comparison is correct
			if module == moduleInScope {
				correctInstance = instance
				break
			}
		}
		// Break outer loop too if an instance was found
		if correctInstance != hotstuff.ZeroInstance {
			break
		}
	}

	// If this variable remained unchanged, return false
	if correctInstance == hotstuff.ZeroInstance {
		return false
	}

	correctScope := mods.scopedModules[correctInstance]
	for _, m := range correctScope {
		mv := reflect.ValueOf(m)
		if mv.Type().AssignableTo(pt.Elem()) {
			v.Elem().Set(mv)
			return true
		}
	}

	return false
}

// MatchForScope assigns ptr to a matching module in the scope with scope.
func (core *Core) MatchForScope(instance hotstuff.Instance, ptr any) {
	if len(core.scopedModules) == 0 {
		panic("pipelining is not enabled")
	}

	v := reflect.ValueOf(ptr)
	if !v.IsValid() {
		panic("pointer value cannot be nil")
	}

	pt := v.Type()
	if pt.Kind() != reflect.Ptr {
		panic("only pointer value allowed")
	}

	scope := core.scopedModules[instance]
	for _, m := range scope {
		mv := reflect.ValueOf(m)
		if mv.Type().AssignableTo(pt.Elem()) {
			v.Elem().Set(mv)
			return
		}
	}

	panic("no match found for " + pt.Elem().Name())
}

// Return the number of scopes the builder has generated.
func (core *Core) ScopeCount() int {
	return len(core.scopedModules)
}

// Return a slice of Scopes in the order which the scopes were created by Builder.
func (core *Core) Scopes() (ids []hotstuff.Instance) {
	for id := range core.scopedModules {
		ids = append(ids, id)
	}

	return
}

// Return a list of modules from a scope. The order of module types is influenced
// by when AddScoped was called in Builder.
func (core *Core) GetScope(instance hotstuff.Instance) []any {
	return core.scopedModules[instance]
}

// Builder is a helper for setting up client modules.
type Builder struct {
	core                 Core
	staticModules        []Module
	moduleScopes         map[hotstuff.Instance]moduleScope
	opts                 *Options
	pipeliningEnabled    bool
	consensusInstanceIds []hotstuff.Instance
}

// NewBuilder returns a new builder.
func NewBuilder(id hotstuff.ID, pk hotstuff.PrivateKey) Builder {
	bl := Builder{
		opts: &Options{
			id:                 id,
			privateKey:         pk,
			connectionMetadata: make(map[string]string),
		},
		pipeliningEnabled:    false,
		consensusInstanceIds: nil,
		moduleScopes:         nil,
	}

	return bl
}

// EnablePipelining enables pipelining by allocating the module scopes and assigning them the instance identifiers.
func (bl *Builder) EnablePipelining(instanceCount int) {
	if bl.pipeliningEnabled {
		panic("pipelining already enabled")
	}

	if instanceCount <= 0 {
		panic("pipelining requires at least one consensus instance")
	}

	bl.pipeliningEnabled = true
	bl.core.scopedModules = make(map[hotstuff.Instance][]any)
	bl.moduleScopes = make(map[hotstuff.Instance]moduleScope)

	for i := hotstuff.Instance(1); i <= hotstuff.Instance(instanceCount); i++ {
		bl.consensusInstanceIds = append(bl.consensusInstanceIds, i)
		bl.moduleScopes[i] = make(moduleScope, 0)
		bl.core.scopedModules[i] = make([]any, 0)
	}

}

// Options returns the options module.
func (b *Builder) Options() *Options {
	return b.opts
}

// Add adds existing, singular, module instances to the builder.
func (b *Builder) Add(modules ...any) {
	b.core.staticModules = append(b.core.staticModules, modules...)
	for _, module := range modules {
		if m, ok := module.(Module); ok {
			b.staticModules = append(b.staticModules, m)
		}
	}
}

// CreateScope constructs N modules of the same type based on the constructor function
// and adds them to a map where each module is mapped to a scope. The module's constructor
// is called with the variadic arguments.
// If pipelining is disabled when CreateScope is called, only one module will be constructed and mapped
// to hotstuff.ZeroInstance.
// To add the modules, use AddScopedModules later.
func (b *Builder) CreateScope(ctor any, ctorArgs ...any) (scopedMods map[hotstuff.Instance]any) {
	if reflect.TypeOf(ctor).Kind() != reflect.Func {
		panic("first argument is not a function")
	}

	scopedMods = make(map[hotstuff.Instance]any)

	vargs := make([]reflect.Value, len(ctorArgs))
	for n, v := range ctorArgs {
		vargs[n] = reflect.ValueOf(v)
	}

	ctorVal := reflect.ValueOf(ctor)
	scopeIds := b.consensusInstanceIds
	if !b.pipeliningEnabled {
		scopeIds = []hotstuff.Instance{hotstuff.ZeroInstance}
	}
	for _, id := range scopeIds {
		returnResult := ctorVal.Call(vargs)
		if len(returnResult) != 1 {
			panic("constructor does not return a single value")
		}
		mod := returnResult[0].Interface()
		scopedMods[id] = mod
	}
	return scopedMods
}

func (b *Builder) AddScope(scopedModuleMaps ...map[hotstuff.Instance]any) {
	for _, scopedMods := range scopedModuleMaps {
		if !b.pipeliningEnabled {
			mod, ok := scopedMods[hotstuff.ZeroInstance]
			if !ok {
				panic("map of scoped modules did not contain zero-instance key")
			}
			b.Add(mod)
			continue
		}

		for id := range scopedMods {
			mod := scopedMods[id]
			converted, ok := mod.(Module)

			b.core.scopedModules[id] = append(b.core.scopedModules[id], mod)
			if !ok {
				continue
			}
			b.moduleScopes[id] = append(b.moduleScopes[id], converted)
		}
	}
}

// Build initializes all added modules and returns the Core object.
func (b *Builder) Build() *Core {
	// reverse the order of the added modules so that TryGet will find the latest first.
	for i, j := 0, len(b.core.staticModules)-1; i < j; i, j = i+1, j-1 {
		b.core.staticModules[i], b.core.staticModules[j] = b.core.staticModules[j], b.core.staticModules[i]
	}
	// add the Options last so that it can be overridden by user.
	b.Add(b.opts)
	opt := InitOptions{
		IsPipeliningEnabled:     b.pipeliningEnabled,
		ModuleConsensusInstance: hotstuff.ZeroInstance,
		InstanceCount:           len(b.consensusInstanceIds),
	}
	for _, module := range b.staticModules {
		module.InitModule(&b.core, opt)
	}

	if !b.pipeliningEnabled {
		b.core.isPipeliningEnabled = false
		return &b.core // Exit early
	}

	b.core.isPipeliningEnabled = true

	// Initializing later so that modules can reference
	// other modules in the same pipe without panicking.
	for instance, scope := range b.moduleScopes {
		opt := InitOptions{
			IsPipeliningEnabled:     b.pipeliningEnabled,
			ModuleConsensusInstance: instance,
			InstanceCount:           len(b.consensusInstanceIds),
		}
		for _, module := range scope {
			module.(Module).InitModule(&b.core, opt)
		}
	}
	return &b.core
}
