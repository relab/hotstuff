package cmdcache

type Option func(*Cache)

func WithBatching(batchSize uint32) Option {
	if batchSize == 0 {
		panic("batch size must be higher than zero")
	}
	return func(cmdCache *Cache) {
		cmdCache.batchSize = int(batchSize)
	}
}
