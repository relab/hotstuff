package hotstuff

import (
	"context"
	"math"
	"time"
)

// Pacemaker is a mechanism that provides synchronization
type Pacemaker interface {
	GetLeader(int) ReplicaID
	Run()
}

// FixedLeaderPacemaker uses a fixed leader.
type FixedLeaderPacemaker struct {
	HS        *HotStuff
	Leader    ReplicaID
	OldLeader ReplicaID
	Commands  chan []byte
}

// GetLeader returns the fixed ID of the leader
func (p FixedLeaderPacemaker) GetLeader(vHeight int) ReplicaID {
	return p.Leader
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

// RoundRobinPacemaker change leader in a RR fashion. The amount of commands to be executed before it changes leader can be customized.
type RoundRobinPacemaker struct {
	HS       *HotStuff
	Commands chan []byte

	TermLength   int
	Schedule     []ReplicaID
	cancel       func()
	newViewCount int
}

// GetLeader returns the fixed ID of the leader for the view height vHeight
func (p *RoundRobinPacemaker) GetLeader(vHeight int) ReplicaID {
	term := int(math.Ceil(float64(vHeight)/float64(p.TermLength)) - 1)
	return p.Schedule[term%len(p.Schedule)]
}

// Beat make the leader brodcast a new proposal for a node to work on.
func (p *RoundRobinPacemaker) Beat() {
	logger.Println("Beat")
	cmd, ok := <-p.Commands
	if !ok {
		p.HS.Close()
		return
	}
	p.HS.Propose(cmd)
}

// Run runs the pacemaker which will beat when the previous QC is completed
func (p *RoundRobinPacemaker) Run() {
	notify := p.HS.GetNotifier()
	if p.HS.id == p.GetLeader(1) {
		go p.Beat()
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.newViewTimeout(ctx, p.HS.timeout)

	for n := range notify {
		switch n.Event {
		case ReceiveProposal:
			p.cancel()
		case QCFinish:
			if p.HS.id == p.GetLeader(p.HS.vHeight+1) {
				go p.Beat()
			} else if p.HS.id == p.GetLeader(p.HS.vHeight) {
				// was leader for previous view, but not the leader for next view
				// do leader change
				go p.HS.doLeaderChange(p.HS.qcHigh)
			}
		case ReceiveNewView:
			if n.Node != nil && n.Node.Height == p.HS.bLeaf.Height {
				p.newViewCount++
				if p.newViewCount >= p.HS.QuorumSize {
					p.newViewCount = 0
					go p.Beat()
				}
			}

		}
	}
}

func (p *RoundRobinPacemaker) newViewTimeout(ctx context.Context, timeout time.Duration) {
	for {
		select {
		case <-time.After(timeout):
			err := p.HS.SendNewView()
			if err != nil {
				p.HS.vHeight++
			}
		case <-ctx.Done():
		}

		var cancel func()
		ctx, cancel = context.WithCancel(context.Background())
		p.cancel = cancel
	}

}
