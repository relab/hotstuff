package hotstuff

import (
	"context"
	"math"
	"time"
)

// Pacemaker is a mechanism that provides synchronization
type Pacemaker interface {
	Run()
}

// FixedLeaderPacemaker uses a fixed leader.
type FixedLeaderPacemaker struct {
	*HotStuff
	Leader    ReplicaID
	OldLeader ReplicaID
	Commands  chan []byte
}

// getLeader returns the fixed ID of the leader
func (p FixedLeaderPacemaker) getLeader(vHeight int) ReplicaID {
	return p.Leader
}

// beat make the leader brodcast a new proposal for a node to work on.
func (p FixedLeaderPacemaker) beat() {
	p.OldLeader = p.Leader
	logger.Println("beat")
	cmd, ok := <-p.Commands
	if !ok {
		// no more commands. Time to quit
		p.Close()
		return
	}
	p.Propose(cmd)
}

// Run runs the pacemaker which will beat when the previous QC is completed
func (p FixedLeaderPacemaker) Run() {
	notify := p.GetNotifier()
	if p.id == p.Leader {
		go p.beat()
	}
	for n := range notify {
		switch n.Event {
		case QCFinish:
			if p.id == p.Leader {
				go p.beat()
			}
		}
	}
}

// RoundRobinPacemaker change leader in a RR fashion. The amount of commands to be executed before it changes leader can be customized.
type RoundRobinPacemaker struct {
	*HotStuff

	Commands   chan []byte
	TermLength int
	Schedule   []ReplicaID

	cancel func() // resets the current new-view interrupt
}

// getLeader returns the fixed ID of the leader for the view height vHeight
func (p *RoundRobinPacemaker) getLeader(vHeight int) ReplicaID {
	term := int(math.Ceil(float64(vHeight)/float64(p.TermLength)) - 1)
	return p.Schedule[term%len(p.Schedule)]
}

// beat make the leader brodcast a new proposal for a node to work on.
func (p *RoundRobinPacemaker) beat() {
	logger.Println("beat: height ", p.height()+1)
	cmd, ok := <-p.Commands
	if !ok {
		p.Close()
		return
	}
	p.Propose(cmd)
}

func (p *RoundRobinPacemaker) height() int {
	return p.bLeaf.Height
}

// Run runs the pacemaker which will beat when the previous QC is completed
func (p *RoundRobinPacemaker) Run() {
	notify := p.GetNotifier()

	// initial beat for view 1
	if p.id == p.getLeader(1) {
		go p.beat()
	}

	// set up new-view interrupt
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.newViewTimeout(ctx, p.timeout)

	// make sure that we only beat once per view, and don't beat if bLeaf.Height < vHeight
	// as that would cause a panic
	lastBeat := 1
	beat := func() {
		if lastBeat < p.height()+1 && p.height()+1 > p.vHeight {
			lastBeat = p.height() + 1
			go p.beat()
		}
	}

	// handle events from hotstuff
	for n := range notify {
		switch n.Event {
		case ReceiveProposal:
			p.cancel()
		case QCFinish:
			if p.id == p.getLeader(p.height()+1) {
				beat()
			} else if p.id == p.getLeader(p.height()) {
				// was leader for previous view, but not the leader for next view
				// do leader change
				go p.SendNewView(p.getLeader(p.height() + 1))
			}
		case ReceiveNewView:
			if n.QC != nil {
				p.UpdateQCHigh(n.QC)
				if p.id == p.getLeader(p.height()+1) {
					beat()
				}
			}
		}
	}
}

func (p *RoundRobinPacemaker) newViewTimeout(ctx context.Context, timeout time.Duration) {
	for {
		select {
		case <-time.After(timeout):
			// add a dummy node to the tree representing this round which failed
			p.bLeaf = createLeaf(p.bLeaf, nil, nil, p.height()+1)
			p.SendNewView(p.getLeader(p.height() + 1))
		case <-ctx.Done():
		}

		var cancel func()
		ctx, cancel = context.WithCancel(context.Background())
		p.cancel = cancel
	}
}
