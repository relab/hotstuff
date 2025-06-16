package clientpb

type CommandCacheOption func(*CommandCache)

func WithBatching(batchSize uint32) CommandCacheOption {
	if batchSize < 2 {
		panic("batch size must at least be two")
	}
	return func(cmdCache *CommandCache) {
		cmdCache.batchSize = batchSize
	}
}
