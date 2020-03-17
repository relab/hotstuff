package pacemaker

import (
	"context"
	"math"
	"time"

	"github.com/relab/hotstuff"
)

// Pacemaker is a mechanism that provides synchronization
type Pacemaker interface {
	Run()
}

// FixedLeaderPacemaker uses a fixed leader.
type FixedLeaderPacemaker struct {
	*hotstuff.HotStuff
	Leader    hotstuff.ReplicaID
	OldLeader hotstuff.ReplicaID
	Commands  chan []byte
}

// getLeader returns the fixed ID of the leader
func (p FixedLeaderPacemaker) getLeader(vHeight int) hotstuff.ReplicaID {
	return p.Leader
}

// beat make the leader brodcast a new proposal for a node to work on.
func (p FixedLeaderPacemaker) beat() {
	p.OldLeader = p.Leader
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
	if p.GetID() == p.Leader {
		go p.beat()
	}
	for n := range notify {
		switch n.Event {
		case hotstuff.QCFinish:
			if p.GetID() == p.Leader {
				go p.beat()
			}
		}
	}
}

// RoundRobinPacemaker change leader in a RR fashion. The amount of commands to be executed before it changes leader can be customized.
type RoundRobinPacemaker struct {
	*hotstuff.HotStuff

	Commands   chan []byte
	TermLength int
	Schedule   []hotstuff.ReplicaID

	NewViewTimeout time.Duration

	cancelTimeout func() // resets the current new-view interrupt
	stopTimeout   func() // stops the new-view interrupts
}

// getLeader returns the fixed ID of the leader for the view height vHeight
func (p *RoundRobinPacemaker) getLeader(vHeight int) hotstuff.ReplicaID {
	term := int(math.Ceil(float64(vHeight)/float64(p.TermLength)) - 1)
	return p.Schedule[term%len(p.Schedule)]
}

// beat make the leader brodcast a new proposal for a node to work on.
func (p *RoundRobinPacemaker) beat() {
	cmd, ok := <-p.Commands
	if !ok {
		p.Close()
		return
	}
	p.Propose(cmd)
}

// Run runs the pacemaker which will beat when the previous QC is completed
func (p *RoundRobinPacemaker) Run() {
	notify := p.GetNotifier()

	// initial beat for view 1
	if p.GetID() == p.getLeader(1) {
		go p.beat()
	}

	// make sure that we only beat once per view, and don't beat if bLeaf.Height < vHeight
	// as that would cause a panic
	lastBeat := 1
	beat := func() {
		if p.getLeader(p.GetHeight()+1) == p.GetID() && lastBeat < p.GetHeight()+1 &&
			p.GetHeight()+1 > p.GetVotedHeight() {
			lastBeat = p.GetHeight() + 1
			go p.beat()
		}
	}

	// get the first notification, thus making sure that leader of view 1 has a chance to beat before timeouts happen
	n := <-notify

	// set up new-view interrupt
	stopContext, cancel := context.WithCancel(context.Background())
	p.stopTimeout = cancel
	cancelContext, cancel := context.WithCancel(context.Background())
	p.cancelTimeout = cancel
	go p.startNewViewTimeout(stopContext, cancelContext)

	// handle events from hotstuff
	for {
		switch n.Event {
		case hotstuff.ReceiveProposal:
			p.cancelTimeout()
		case hotstuff.QCFinish:
			if p.GetID() != p.getLeader(p.GetHeight()+1) {
				// was leader for previous view, but not the leader for next view
				// do leader change
				go p.SendNewView(p.getLeader(p.GetHeight() + 1))
			}
			beat()
		case hotstuff.ReceiveNewView:
			beat()
		}

		var ok bool
		n, ok = <-notify
		if !ok {
			break
		}
	}

	// clean up
	p.stopTimeout()
}

// startNewViewTimeout sends a NewView to the leader if triggered by a timer interrupt. Two contexts are used to control
// this function; the stopContext is used to stop the function, and the cancelContext is used to cancel a single timer.
func (p *RoundRobinPacemaker) startNewViewTimeout(stopContext, cancelContext context.Context) {
	for {
		select {
		case <-stopContext.Done():
			p.cancelTimeout()
			return
		case <-time.After(p.NewViewTimeout):
			// Det ser ut til å være 2 måter å løysa dette på. Metode 1) Spør lederen om den kan sende noden igjenn, eller metode 2) Bare lag en fake QC for å få sendt noden igjennom alle sjekkene.

			// add a dummy node to the tree representing this round which failed
			//p.SetLeafNode(hotstuff.CreateLeaf(p.GetLeafNode(), nil, nil, p.GetHeight()+1))

			// Ask leader to resend the new node
			newNode, _ := p.ResendNode(p.getLeader(p.GetHeight()))

			p.SetLeafNode(newNode)

			p.SendNewView(p.getLeader(p.GetHeight() + 1))
		case <-cancelContext.Done():
		}

		var cancel func()
		cancelContext, cancel = context.WithCancel(context.Background())
		p.cancelTimeout = cancel
	}
}
