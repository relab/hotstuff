// Package gpool provides a generic sync.Pool.
package gpool

import "sync"

// Pool is a generic sync.Pool.
type Pool[T any] sync.Pool

// New returns an initialized generic sync.Pool.
func New[T any](newFunc func() T) Pool[T] {
	if newFunc != nil {
		return Pool[T](sync.Pool{
			New: func() any { return newFunc() },
		})
	}
	return Pool[T]{}
}

// Get retrieves a resource from the pool.
// Returns the zero value of T if no resource is available and no New func is specified.
func (p *Pool[T]) Get() (val T) {
	sp := (*sync.Pool)(p)
	v := sp.Get()
	if v != nil {
		return v.(T)
	}
	return val
}

// Put puts the resource into the pool.
func (p *Pool[T]) Put(val T) {
	sp := (*sync.Pool)(p)
	sp.Put(val)
}
