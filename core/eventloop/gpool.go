package eventloop

import "sync"

// pool is a generic sync.pool.
type pool[T any] sync.Pool

// New returns an initialized generic sync.Pool.
func newPool[T any](newFunc func() T) pool[T] {
	if newFunc != nil {
		return pool[T](sync.Pool{
			New: func() any { return newFunc() },
		})
	}
	return pool[T]{}
}

// Get retrieves a resource from the pool.
// Returns the zero value of T if no resource is available and no New func is specified.
func (p *pool[T]) Get() (val T) {
	sp := (*sync.Pool)(p)
	v := sp.Get()
	if v != nil {
		return v.(T)
	}
	return val
}

// Put puts the resource into the pool.
func (p *pool[T]) Put(val T) {
	sp := (*sync.Pool)(p)
	sp.Put(val)
}
