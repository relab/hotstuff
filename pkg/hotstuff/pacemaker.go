package hotstuff

// Pacemaker is a mechanism that provides synchronization
type Pacemaker interface {
	GetLeader() ReplicaID
}

// FixedLeaderPacemaker uses a fixed leader.
type FixedLeaderPacemaker struct {
	hs     *HotStuff
	leader ReplicaID
}

// GetLeader returns the fixed ID of the leader
func (p FixedLeaderPacemaker) GetLeader() ReplicaID {
	return p.leader
}

// Beat make the leader brodcast a new proposal for a node to work on.
func (p FixedLeaderPacemaker) Beat() {
	if p.hs.id == p.GetLeader() {
		p.hs.bLeaf = p.hs.Propose()
	}
}

// Start runs the pacemaker which will beat when the previous QC is completed
func (p FixedLeaderPacemaker) Start() {
	go p.Beat()
	notify := p.hs.GetNotifier()
	for n := range notify {
		switch n.Event {
		case QCFinish:
			go p.Beat()
		}
	}
}
