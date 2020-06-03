package pacemaker

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/logging"
)

var logger *log.Logger

func init() {
	logger = logging.GetLogger()
}

// FixedLeader uses a fixed leader.
type FixedLeader struct {
	*hotstuff.HotStuff
	leader config.ReplicaID
}

// NewFixedLeader returns a new fixed leader pacemaker
func NewFixedLeader(leaderID config.ReplicaID) *FixedLeader {
	return &FixedLeader{
		leader: leaderID,
	}
}

func (p *FixedLeader) Init(hs *hotstuff.HotStuff) {
	p.HotStuff = hs
}

func (p *FixedLeader) GetLeader() config.ReplicaID {
	return p.leader
}

// Run runs the pacemaker which will beat when the previous QC is completed
func (p *FixedLeader) Run(ctx context.Context) {
	notify := p.GetEvents()
	if p.Config.ID == p.leader {
		logger.Println("Beat")
		go p.Propose()
	}
	var n consensus.Event
	var ok bool
	for {
		select {
		case n, ok = <-notify:
			if !ok {
				return
			}
		case <-ctx.Done():
			return
		}
		switch n {
		case consensus.QCFinish:
			if p.Config.ID == p.leader {
				logger.Println("Beat")
				go p.Propose()
			}
		}
	}
}

// RoundRobin change leader in a RR fashion. The amount of commands to be executed before it changes leader can be customized.
type RoundRobin struct {
	*hotstuff.HotStuff

	termLength int
	schedule   []config.ReplicaID
	timeout    time.Duration

	resetTimer  chan struct{} // sending on this channel will reset the timer
	stopTimeout func()        // stops the new-view interrupts
}

// NewRoundRobin returns a new round robin pacemaker
func NewRoundRobin(termLength int, schedule []config.ReplicaID, timeout time.Duration) *RoundRobin {
	return &RoundRobin{
		termLength: termLength,
		schedule:   schedule,
		timeout:    timeout,
		resetTimer: make(chan struct{}),
	}
}

func (p *RoundRobin) Init(hs *hotstuff.HotStuff) {
	p.HotStuff = hs
}

func (p *RoundRobin) GetLeader() config.ReplicaID {
	return p.getLeader(p.GetHeight())
}

// getLeader returns the fixed ID of the leader for the view height vHeight
func (p *RoundRobin) getLeader(vHeight int) config.ReplicaID {
	term := int(math.Ceil(float64(vHeight) / float64(p.termLength)))
	return p.schedule[term%len(p.schedule)]
}

// Run runs the pacemaker which will beat when the previous QC is completed
func (p *RoundRobin) Run(ctx context.Context) {
	notify := p.GetEvents()

	// initial beat
	if p.getLeader(0) == p.Config.ID {
		go p.Propose()
	}

	// get initial notification
	n := <-notify

	// make sure that we only beat once per view, and don't beat if bLeaf.Height < vHeight
	// as that would cause a panic
	lastBeat := 1
	beat := func() {
		if p.GetLeader() == p.Config.ID && lastBeat < p.GetHeight() &&
			p.GetHeight() >= p.GetVotedHeight() {
			lastBeat = p.GetHeight()
			go p.Propose()
		}
	}

	// set up new-view interrupt
	stopContext, cancel := context.WithCancel(context.Background())
	p.stopTimeout = cancel
	go p.startNewViewTimeout(stopContext)
	defer p.stopTimeout()

	// handle events from hotstuff
	for {
		switch n {
		case consensus.ReceiveProposal:
			p.resetTimer <- struct{}{}
		case consensus.QCFinish:
			beat()
		case consensus.ReceiveNewView:
			beat()
		}

		var ok bool
		select {
		case n, ok = <-notify:
			if !ok {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// startNewViewTimeout sends a NewView to the leader if triggered by a timer interrupt. Two contexts are used to control
// this function; the stopContext is used to stop the function, and the cancelContext is used to cancel a single timer.
func (p *RoundRobin) startNewViewTimeout(stopContext context.Context) {
	for {
		select {
		case <-p.resetTimer:
		case <-stopContext.Done():
			return
		case <-time.After(p.timeout):
			// add a dummy block to the tree representing this round which failed
			logger.Println("NewViewTimeout triggered")
			p.SetLeaf(consensus.CreateLeaf(p.GetLeaf(), nil, nil, p.GetHeight()+1))
			p.SendNewView(p.getLeader(p.GetHeight() + 1))
		}
	}
}
