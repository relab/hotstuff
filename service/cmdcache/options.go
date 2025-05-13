package cmdcache

type Option func(*Cache)

func WithBatching(batchSize uint32) Option {
	if batchSize < 2 {
		panic("batch size must at least be two")
	}
	return func(cmdCache *Cache) {
		cmdCache.batchSize = batchSize
	}
}
