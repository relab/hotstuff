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
	"github.com/relab/hotstuff/pipeline"
)

type ModulePipe []Module

type InitOptions struct {
	IsPipeliningEnabled bool
	PipeCount           int
	ModulePipeId        pipeline.Pipe
}

// Module is an interface for initializing modules.
type Module interface {
	InitModule(mods *Core, buildOpt InitOptions)
}

func NewPiped[T any](pipeCount uint, ctor any, ctorArgs ...any) (pipedMods map[pipeline.Pipe]T) {
	if reflect.TypeOf(ctor).Kind() != reflect.Func {
		panic("first argument is not a function")
	}

	pipedMods = make(map[pipeline.Pipe]T)

	vargs := make([]reflect.Value, len(ctorArgs))
	for n, v := range ctorArgs {
		vargs[n] = reflect.ValueOf(v)
	}

	ctorVal := reflect.ValueOf(ctor)

	if pipeCount == 0 {
		pipeCount = 1
	}
	for pipe := pipeline.Pipe(1); pipe <= pipeline.Pipe(pipeCount); pipe++ {
		returnResult := ctorVal.Call(vargs)
		if len(returnResult) != 1 {
			panic("constructor does not return a single value")
		}
		mod, ok := returnResult[0].Interface().(T)
		if !ok {
			panic("constructor did not return correct value type")
		}
		pipedMods[pipe] = mod
	}
	return pipedMods
}

// Core is the base of the module system.
// It contains only a few core modules that are shared between replicas and clients.
type Core struct {
	staticModules       []any
	pipedModules        map[pipeline.Pipe][]any
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

// GetPiped does the same as Get and additionally searches for pointers in the same pipe as moduleInPipe.
// If pipelining is not enabled, Get is called internally instead.
//
// NOTE: pointers must only contain non-nil pointers to types that have been provided to the module system
// as a piped module.
// GetPiped panics if one of the given arguments is not a pointer, if a compatible module is not found,
// if the module was not in a pipe or if the module was not in the same pipe as moduleInPipe.
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
//		mods.GetPiped(m, &m.otherModule) // Requires an OtherModule from the same pipe
//	}
//
//	func main() {
//		pipeIds := []modules.PipeId{0, 1, 2, ...}
//
//		builder := modules.NewBuilder(0, nil)
//		builder.EnablePipelining(pipeIds)
//		builder.AddPiped(NewMyModuleImpl)
//		builder.AddPiped(NewOtherModuleImpl)
//		builder.Build() // InitModule is called here
//	}
func (mods *Core) GetPiped(moduleInPipe Module, pointers ...any) {
	if len(pointers) == 0 {
		panic("no pointers given")
	}

	if !mods.isPipeliningEnabled {
		mods.Get(pointers...)
		return
	}

	for _, ptr := range pointers {
		if !mods.TryGet(ptr) {
			if !mods.tryGetFromPipe(moduleInPipe, ptr) {
				panic(fmt.Sprintf("piped module of type %s not found", reflect.TypeOf(ptr).Elem()))
			}
		}
	}
}

// TryGetFromPipe attempts to find a module for ptr which also happens to be in the same
// pipe as moduleInPipe, false otherwise.
// TryGetFromPipe returns true if a module was successflully stored in ptr, false otherwise.
// If pipelining was not enabled, TryGet is called implicitly.
func (mods *Core) tryGetFromPipe(moduleInPipe Module, ptr any) bool {
	if len(mods.pipedModules) == 0 {
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

	correctPipeId := pipeline.NullPipe
	for id := range mods.pipedModules {
		pipe := mods.pipedModules[id]
		// Check if self is in pipe
		for _, module := range pipe {
			// TODO: Verify if equality comparison is correct
			if module == moduleInPipe {
				correctPipeId = id
				break
			}
		}
		// Break outer loop too if a pipe ID was found
		if correctPipeId != pipeline.NullPipe {
			break
		}
	}

	// If this variable remained unchanged, return false
	if correctPipeId == pipeline.NullPipe {
		return false
	}

	correctPipe := mods.pipedModules[correctPipeId]
	for _, m := range correctPipe {
		mv := reflect.ValueOf(m)
		if mv.Type().AssignableTo(pt.Elem()) {
			v.Elem().Set(mv)
			return true
		}
	}

	return false
}

// MatchForPipe assigns ptr to a matching module in the pipe with pipeId.
func (core *Core) MatchForPipe(pipeId pipeline.Pipe, ptr any) {
	if len(core.pipedModules) == 0 {
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

	pipe := core.pipedModules[pipeId]
	for _, m := range pipe {
		mv := reflect.ValueOf(m)
		if mv.Type().AssignableTo(pt.Elem()) {
			v.Elem().Set(mv)
			return
		}
	}

	panic("no match found for " + pt.Elem().Name())
}

// Return the number of pipes the builder has generated.
func (core *Core) PipeCount() int {
	return len(core.pipedModules)
}

// Return a slice of Pipes in the order which the pipes were created by Builder.
func (core *Core) Pipes() (ids []pipeline.Pipe) {
	for id := range core.pipedModules {
		ids = append(ids, id)
	}

	return
}

// Return a list of modules from a pipes. The order of module types is influenced
// by when AddPiped was called in Builder.
func (core *Core) GetPipe(id pipeline.Pipe) []any {
	return core.pipedModules[id]
}

// Builder is a helper for setting up client modules.
type Builder struct {
	core              Core
	staticModules     []Module
	modulePipes       map[pipeline.Pipe]ModulePipe
	opts              *Options
	pipeliningEnabled bool
	pipeIds           []pipeline.Pipe
}

// NewBuilder returns a new builder.
func NewBuilder(id hotstuff.ID, pk hotstuff.PrivateKey) Builder {
	bl := Builder{
		opts: &Options{
			id:                 id,
			privateKey:         pk,
			connectionMetadata: make(map[string]string),
		},
		pipeliningEnabled: false,
		pipeIds:           nil,
		modulePipes:       nil,
	}

	return bl
}

// EnablePipelining enables pipelining by allocating the module pipes and assigning them the ids
// provided by pipeIds. The number of pipes will be len(pipeIds).
func (bl *Builder) EnablePipelining(pipeCount int) {
	if bl.pipeliningEnabled {
		panic("pipelining already enabled")
	}

	if pipeCount <= 0 {
		panic("pipelining requires at least one pipe")
	}

	bl.pipeliningEnabled = true
	bl.core.pipedModules = make(map[pipeline.Pipe][]any)
	bl.modulePipes = make(map[pipeline.Pipe]ModulePipe)

	for pipe := pipeline.Pipe(1); pipe <= pipeline.Pipe(pipeCount); pipe++ {
		bl.pipeIds = append(bl.pipeIds, pipe)
		bl.modulePipes[pipe] = make(ModulePipe, 0)
		bl.core.pipedModules[pipe] = make([]any, 0)
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

// CreatePiped constructs N modules of the same type based on the constructor function
// and adds them to a map where each module is mapped to a pipe. The module's constructor
// is called with the variadic arguments.
// If pipelining is disabled when CreatePiped is called, only one module will be constructed and mapped
// to pipeline.NullPipe.
// To add the modules, use AddPipedModules later.
func (b *Builder) CreatePiped(ctor any, ctorArgs ...any) (pipedMods map[pipeline.Pipe]any) {
	if reflect.TypeOf(ctor).Kind() != reflect.Func {
		panic("first argument is not a function")
	}

	pipedMods = make(map[pipeline.Pipe]any)

	vargs := make([]reflect.Value, len(ctorArgs))
	for n, v := range ctorArgs {
		vargs[n] = reflect.ValueOf(v)
	}

	ctorVal := reflect.ValueOf(ctor)
	pipes := b.pipeIds
	if !b.pipeliningEnabled {
		pipes = []pipeline.Pipe{pipeline.NullPipe}
	}
	for _, id := range pipes {
		returnResult := ctorVal.Call(vargs)
		if len(returnResult) != 1 {
			panic("constructor does not return a single value")
		}
		mod := returnResult[0].Interface()
		pipedMods[id] = mod
	}
	return pipedMods
}

func (b *Builder) AddPiped(pipedModMaps ...map[pipeline.Pipe]any) {
	for _, pipedMods := range pipedModMaps {
		if !b.pipeliningEnabled {
			mod, ok := pipedMods[pipeline.NullPipe]
			if !ok {
				panic("map of piped modules did not contain null-pipe key")
			}
			b.Add(mod)
			continue
		}

		for id := range pipedMods {
			mod := pipedMods[id]
			converted, ok := mod.(Module)

			b.core.pipedModules[id] = append(b.core.pipedModules[id], mod)
			if !ok {
				continue
			}
			b.modulePipes[id] = append(b.modulePipes[id], converted)
		}
	}
}

// AddPiped constructs and adds n instances of a module kind, provided its constructor and subsequent
// constructor arguments. If pipelining is not enabled, only one will be created and Add is called for it.
/*func (b *Builder) AddPiped(ctor any, ctorArgs ...any) {
	if reflect.TypeOf(ctor).Kind() != reflect.Func {
		panic("first argument is not a function")
	}

	vargs := make([]reflect.Value, len(ctorArgs))
	for n, v := range ctorArgs {
		vargs[n] = reflect.ValueOf(v)
	}

	ctorVal := reflect.ValueOf(ctor)
	if !b.pipeliningEnabled {
		returnResult := ctorVal.Call(vargs)
		if len(returnResult) != 1 {
			panic("constructor does not return a single value")
		}
		mod := returnResult[0].Interface()
		b.Add(mod)
		return
	}

	for id := range b.modulePipes {
		returnResult := ctorVal.Call(vargs)
		if len(returnResult) != 1 {
			panic("constructor does not return a single value")
		}
		mod := returnResult[0].Interface()
		converted, ok := mod.(Module)

		b.core.pipedModules[id] = append(b.core.pipedModules[id], mod)
		if !ok {
			continue
		}
		b.modulePipes[id] = append(b.modulePipes[id], converted)
	}
}*/

// Build initializes all added modules and returns the Core object.
func (b *Builder) Build() *Core {
	// reverse the order of the added modules so that TryGet will find the latest first.
	for i, j := 0, len(b.core.staticModules)-1; i < j; i, j = i+1, j-1 {
		b.core.staticModules[i], b.core.staticModules[j] = b.core.staticModules[j], b.core.staticModules[i]
	}
	// add the Options last so that it can be overridden by user.
	b.Add(b.opts)
	opt := InitOptions{
		IsPipeliningEnabled: b.pipeliningEnabled,
		ModulePipeId:        pipeline.NullPipe,
		PipeCount:           len(b.pipeIds),
	}
	for _, module := range b.staticModules {
		module.InitModule(&b.core, opt)
	}

	if !b.pipeliningEnabled {
		b.core.isPipeliningEnabled = false
		return &b.core // Exit early
	}

	b.core.isPipeliningEnabled = true
	// Adding the piped modules to core first.
	// b.core.pipedModules = make(map[pipelining.PipeId][]any)
	// for id, pipe := range b.modulePipes {
	// 	b.core.pipedModules[id] = make([]any, 0)
	// 	for _, module := range pipe {
	// 		b.core.pipedModules[id] = append(b.core.pipedModules[id], module)
	// 	}
	// }

	// Initializing later so that modules can reference
	// other modules in the same pipe without panicking.
	for pipeId, pipe := range b.modulePipes {
		pipeOpt := InitOptions{
			IsPipeliningEnabled: b.pipeliningEnabled,
			ModulePipeId:        pipeId,
			PipeCount:           len(b.pipeIds),
		}
		for _, module := range pipe {
			module.(Module).InitModule(&b.core, pipeOpt)
		}
	}
	return &b.core
}
