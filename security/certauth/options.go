package certauth

import "container/list"

type Option func(*CertAuthority)

// WithCache wraps the existing CryptoBase implementation with one that caches the results of the crypto operations.
func WithCache(cacheSize int) Option {
	if cacheSize <= 0 {
		panic("cache size cannot be zero")
	}
	return func(ca *CertAuthority) {
		prevImpl := ca.CryptoBase
		ca.CryptoBase = &Cache{
			impl:     prevImpl,
			capacity: cacheSize,
			entries:  make(map[string]*list.Element, cacheSize),
		}
	}
}
