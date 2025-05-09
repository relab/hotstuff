package clientsrv

type CacheOption func(*CmdCache)

func WithBatching(batchSize uint32) CacheOption {
	if batchSize == 1 {
		panic("batch size must be higher than one")
	}
	return func(cmdCache *CmdCache) {
		cmdCache.batchSize = int(batchSize)
	}
}
