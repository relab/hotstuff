package hotstuff

// Pacemaker is a mechanism that provides synchronization
type Pacemaker interface {
	GetLeader() int
	Beat(cmd []byte)
	NextSyncView()
	UpdateQCHigh()
}
