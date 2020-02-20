package hotstuff

// Pacemaker is a mechanism that provides synchronization
type Pacemaker interface {
	GetLeader() ReplicaID
	GetOldLeader() ReplicaID
}

// FixedLeaderPacemaker uses a fixed leader.
type FixedLeaderPacemaker struct {
	HS        *HotStuff
	Leader    ReplicaID
	OldLeader ReplicaID
	Commands  chan []byte
}

// RoundRobinPacemaker change leader in a RR fashion. The amount of commands to be executed before it changes leader can be customized.
type RoundRobinPacemaker struct {
	AmountOfCommandsPerLeader int
	ReplicaSlice              []*HotStuff
	IndexOfCurrentLeader      int
	HS                        *HotStuff
	Leader                    ReplicaID
	OldLeader                 ReplicaID
	Commands                  chan []byte
}

func (p *RoundRobinPacemaker) changeLeader() {
	p.OldLeader = p.Leader
	p.IndexOfCurrentLeader = (p.IndexOfCurrentLeader + 1) % len(p.ReplicaSlice)
	p.Leader = p.ReplicaSlice[p.IndexOfCurrentLeader].id
}

// GetLeader returns the fixed ID of the leader
func (p FixedLeaderPacemaker) GetLeader() ReplicaID {
	return p.Leader
}

// GetLeader returns the fixed ID of the leader
func (p RoundRobinPacemaker) GetLeader() ReplicaID {
	return p.Leader
}

// GetOldLeader returns the fixed ID of the immediate predecessor to the current leader.
func (p FixedLeaderPacemaker) GetOldLeader() ReplicaID {
	return p.OldLeader
}

// GetOldLeader returns the fixed ID of the immediate predecessor to the current leader.
func (p RoundRobinPacemaker) GetOldLeader() ReplicaID {
	return p.OldLeader
}

// Beat make the leader brodcast a new proposal for a node to work on.
func (p FixedLeaderPacemaker) Beat() {
	p.OldLeader = p.Leader
	logger.Println("Beat")
	cmd, ok := <-p.Commands
	if !ok {
		// no more commands. Time to quit
		p.HS.Close()
		return
	}
	p.HS.Propose(cmd)
}

// Beat make the leader brodcast a new proposal for a node to work on.
func (p RoundRobinPacemaker) Beat() {
	logger.Println("Beat")
	cmd, ok := <-p.Commands
	if !ok {
		p.HS.Close()
		return
	}
	p.HS.Propose(cmd)
}

// Run runs the pacemaker which will beat when the previous QC is completed
func (p FixedLeaderPacemaker) Run() {
	notify := p.HS.GetNotifier()
	if p.HS.id == p.Leader {
		go p.Beat()
	}
	for n := range notify {
		switch n.Event {
		case QCFinish:
			if p.HS.id == p.Leader {
				go p.Beat()
			}
		}
	}
}

// Run runs the pacemaker which will beat when the previous QC is completed
func (p RoundRobinPacemaker) Run() {
	notify := p.HS.GetNotifier()
	if p.HS.id == p.Leader {
		go p.Beat()
	}

	for i, rep := range p.ReplicaSlice {
		if rep.id == p.Leader {
			p.IndexOfCurrentLeader = i
		}
	}

	progressNumber := 0

	for n := range notify {
		switch n.Event {
		case QCFinish:
			if p.HS.id == p.Leader {
				go p.Beat()
			}
		case ReceiveProposal:
			if p.AmountOfCommandsPerLeader <= progressNumber {
				p.changeLeader()
				progressNumber = 0
			} else {
				progressNumber++
			}
		}
	}
}
