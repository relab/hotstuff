# Challenges of a Modular BFT Consensus Implementation

## Contents

- [Challenges of a Modular BFT Consensus Implementation](#challenges-of-a-modular-bft-consensus-implementation)
  - [Contents](#contents)
  - [Introduction](#introduction)
  - [The Circular Dependency Problem](#the-circular-dependency-problem)
    - [The Best Practices Solution](#the-best-practices-solution)
    - [Event-based Indirection](#event-based-indirection)
    - [Deferred Initialization](#deferred-initialization)
  - [The Composition Problem](#the-composition-problem)
    - [The Module System](#the-module-system)
    - [A Module Registry](#a-module-registry)
  - [Conclusion](#conclusion)

## Introduction

This project, called `hotstuff`, started out as an implementation of the HotStuff BFT consensus protocol.
Over time, however, the project has evolved to include implementations of other variants of the HotStuff protocol,
as well as other protocols solving different problems, such as the view synchronization problem (see the view synchronizer),
and scaling problems (see Handel).

Writing implementations of these protocols is one thing, but the protocols should also be able to function in practice,
interoperating to achieve BFT consensus.
Each protocol may depend upon other components (we will call them modules) in order to perform its functions.
For example, the core consensus protocol (that is HotStuff and its variants) depends on a view synchronizer module (also known as a pacemaker).
Our implementation of the consensus protocol requires the following features of the synchronizer module:

- Keeps track of the current view (based on the highest known QC, etc.).
- Eventually triggers a proposal in the leader of a view.
- Eventually raises a timeout if progress in the current view has stalled.

These requirements inform our implementation of the view synchronizer and helps us to design an interface that the consensus protocol can use to interoperate with it.

When implementing this system, we run into a number of programming challenges.
In this document, we will discuss these problems and assess different solutions to them.

## The Circular Dependency Problem

In implementing a system that is modular in the way described above where each module may have multiple different implementations and may also depend on several other such components,
we run into problems when trying to initialize the modules:
To initialize our consensus module, we need to initialize our synchronizer module.
However, our synchronizer module might also depend on our consensus module,
depending on which synchronizer implementation is used.
Here we have a chicken and egg situation.

### The Best Practices Solution

Due to the problem described above, as well as other problems, circular dependencies in code are considered bad practice.
Hence, following the "best practices" would call for refactoring of the code to avoid circular dependencies.
Unfortunately, this solution places challenging constraints on what we can do when creating new implementations of a module.
For example, our first implementation of a leader rotation scheme, the round-robin scheme, has no dependencies.
But later, when we want to implement on a more dynamic leader rotation scheme,
we realize that we need historical information about the previous views and thus need to interact with the synchronizer and blockchain modules.
The synchronizer probably needs to know about the identity of the leader, so we now have to refactor these somehow.

It seems then, that strict adherence to the best practice of avoiding circular dependencies is only going to complicate the matter of creating new module implementations.
Of course, we should keep this best practice in mind, but we should allow circular dependencies in order to make it easier to implement new modules.

### Event-based Indirection

Event-based indirection is not a good solution, but it warrants discussion anyway.
The idea is to use the event system that we have developed to facilitate communication between modules.
Instead of each module having a direct dependency on other modules,
it would depend upon the event system to deliver some request to another module.
The other module would then process the request and emit an event containing the result.

The problems of event-based indirection are:
First, it is less efficient than calling an interface method.
Second, it introduces a whole host of implementation challenges.

However, interactions between modules are already in the form of events.
For example, the delivery of messages from the network, or the occurrence of a timeout or proposal.
Therefore, it may make sense to use this solution for some interactions between modules,
but it is not sensible to use this as a solution to the circular dependency problem.

### Deferred Initialization

This idea is simple: First create the instances of all the modules.
Afterwards, initialize the modules using the now existing, albeit uninitialized, modules.
Consider the following code example:

```go
type A struct { b *B }
func NewA() *A { return &A{} }
func (a *A) Init(*b B) { a.b = b }

type B struct { a *A }
func NewB() *B { return &B{} }
func (b *B) Init(*a A) { b.a = a }

func main() {
  a, b := NewA(), NewB()
  a.Init(b)
  b.Init(a)
}
```

Here we are able to initialize both the `A` and `B` modules which are mutually dependent by deferring their full initialization until after they have been constructed.
This solution works well as long as the modules can exist in an "uninitialized" state.
The solution we have implemented is based on this idea, but it is also connected to our solution to the next problem.

## The Composition Problem

Another problem related to the initialization of our modules is this:
How do we build a composition of modules based on a requested configuration.
In other words, how do we select a certain subset of the available modules and initialize them?

For example, let's say that we want to initialize a system with two kinds of modules.
That is, two different module interfaces.
Let us call them A and B.
Interface A is implemented by modules A1 and A2.
Interface B is implemented by module B1 only.
We will use an arrow notation to express dependency.
Let us say that A1 depends on B, and A2 and B1 are independent.
If A1 is chosen, then an implementation of B must be provided to A1.
However, if A2 is chosen, it does not need a B implementation in order to work.

A naive implementation of this system could be done like this:

```go
func compose(choiceA string) {
  var (
    a A
    b B
  )
  b = NewB1()
  if choiceA == "A1" {
    a = NewA1(b)
  } else {
    a = NewA2()
  }
}
```

This naive implementation is not very good because it is difficult to extend when adding new modules.
Also note that this does not include the solution to the circular dependency problem discussed above.
Implementing deferred initialization as discussed above would further extend the amount of boilerplate code in the naive implementation.

### The Module System

Our solution to the composition problem and the circular dependency problem is the *module system*.
It is essentially a combination of dependency injection and deferred initialization.
In short, it is a set of interfaces and data structures that simplifies the composition of modules.
The basic idea is this:

Each module may implement the following `Module` interface.

```go
type Module interface {
  InitModule(mods *modules.Core)
}
```

The `InitModule` method is called by the module system to give the module an opportunity to initialize itself.
The module does this by calling the `Get`, `GetAll` or `TryGet` methods of the `modules.Core` object.
These methods take a pointer to the variable where a module should be stored.
The module system then looks for a module of the requested type and stores it in the pointer.

For example:

```go
type A1 struct{ b B }

func (a *A1) InitModule(mods *modules.Core) {
  mods.Get(&a.b)
}
```

The module system collects a list of modules passed to a `modules.Builder` object.
When the builder's `Build` method is called, all modules added to the builder are initialized,
if they implement the `Module` interface.

But how does the module system know what interface a module implements?
How does the `Get` method find the correct module to store in the pointer?
Either the modules must explicitly declare what type they want to provide to other modules,
or we could simply check all registered modules to see if any of them "fit" in the pointer.
Go's `reflect` package supports this via a `Type.AssignableTo` method.
For now, this is what we have implemented, as it feels more natural in Go which already has implicit interfaces.
However, an explicit version could work like this:



A `Provider` interface has a single method called `ModuleType()`:

```go
type Provider interface {
  ModuleType() any
}
```

This method should return a pointer to the type (typically an interface) that the module provides to other modules.
For a module implementing interface A, this method should simply return `new(A)`.
Additionally, we can make it really easy to implement this interface by using generics:

```go
type Implements[T any] struct{}

func (Implements[T]) ModuleType() any {
  return new(T)
}
```

By embedding `modules.Implements[T]` where T specifies the module's interface,
the module system can become aware of which modules provide what interfaces.
Note that this does not ensure that the module in fact implements `T`,
it is more like an annotation that promises to the module system that `T` is implemented.

Now we have most of the solution. To compare with the naive code, we now have this:

```go
func compose(choiceA string) {
  builder := modules.NewBuilder()
  builder.Add(NewB1())
  if choiceA == "A1" {
    builder.Add(NewA1())
  } else {
    builder.Add(NewA2())
  }
  mods := builder.Build()

  var (
    a A
    b B
  )
  mods.GetAll(&a, &b)
}
```

This doesn't really look a lot better than the naive code, but note the following:

- Circular dependencies are now supported.
- We did not have to specify in the composition method that A1 depends on interface B.

### A Module Registry

To remove the need for an if/else or switch statement in our compose function, we add a *module registry*.
This registry maps the constructor for each module to the module's name.
Each module must register itself with the registry in order to become available.

Modules register themselves by calling a global `modules.Register` function, providing its name and constructor.
For example:

```go
func init() {
  modules.RegisterModule("A1", NewA1)
}
```

Then, a module can be constructed by calling `modules.New` with the name of the requested module.

After adding the module registry, our compose function looks like this:

```go
func compose(choiceA string) {
  builder := modules.NewBuilder()
  builder.Add(
    NewB1(),
    modules.New(choiceA),
  )
  mods := builder.Build()

  var (
    a A
    b B
  )
  mods.GetAll(&a, &b)
}
```

Now, the code is very easy to extend.
If we want to add a new module, we write our implementation,
ensure it implements the `Provider` and `Module` interfaces as necessary, and then register it in the module registry.

## Conclusion

This document has described two problems with implementing a modular system like `hotstuff`.
Namely, the circular dependency problem and the composition problem.
These both relate to the initialization of the system.
First, how do we initialize modules that are mutually dependent?
We have seen that requiring all circular dependencies to be removed makes it difficult to implement new modules.
We have seen that an indirection approach using the event system is too complicated.
In the end, we conclude that deferred initialization is the simplest of the three solutions.
Second, how do we compose together the specific module implementations that we want?
We have seen how the module system and module registry greatly simplify this process.
