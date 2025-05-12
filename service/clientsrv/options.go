package clientsrv

type CacheOption func(*CmdCache)

func WithBatching(batchSize uint32) CacheOption {
	if batchSize == 0 {
		panic("batch size must be higher than zero")
	}
	return func(cmdCache *CmdCache) {
		cmdCache.batchSize = int(batchSize)
	}
}
