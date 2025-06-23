package cert

import "container/list"

type Option func(*Authority)

// WithCache wraps the existing CryptoBase implementation with one that caches the results of the crypto operations.
func WithCache(cacheSize int) Option {
	if cacheSize <= 0 {
		panic("cache size cannot be zero")
	}
	return func(ca *Authority) {
		prevImpl := ca.Base
		ca.Base = &Cache{
			impl:     prevImpl,
			capacity: cacheSize,
			entries:  make(map[string]*list.Element, cacheSize),
		}
	}
}
