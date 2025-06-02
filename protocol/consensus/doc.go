// Package consensus defines the types and interfaces that are used to implement consensus.
//
// In particular, this package defines the module system that allows the different components in a consensus protocol
// to interact with each other. This is an extension of the module system in the modules package,
// and thus any module that is written for the modules package can be used as a module with this package's module system,
// but not vice-versa.
//
// The module system in this package nearly identical to the modules package, except it includes many more modules,
// and it adds a small configuration component, allowing modules to set configuration options at runtime.
// Thus, to write a module for the consensus.Modules system, you must instead implement the consensus.Module interface.
// Refer to the documentation of the modules package for a more detailed description of the module system.
//
// This package also provides a default implementation of the Consensus interface that can be used by
// implementors of the Rules interface. This results in a clean and simple way to implement a new consensus protocol
// while sharing a lot of the code.
package consensus
