// Package synchronizer implements the synchronizer module.
package synchronizer

import (
	"bytes"
	"context"
	"time"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/security/cert"

	"github.com/relab/hotstuff"
)

// Synchronizer implements the DiemBFT pacemaker and is the main loop of the consensus protocol.
type Synchronizer struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
	config    *core.RuntimeConfig

	auth *cert.Authority

	duration       ViewDuration
	leaderRotation leaderrotation.LeaderRotation
	timeoutRules   TimeoutRuler
	voter          *consensus.Voter
	proposer       *consensus.Proposer
	state          *protocol.ViewStates

	sender core.Sender

	// A pointer to the last timeout message that we sent.
	// If a timeout happens again before we advance to the next view,
	// we will simply send this timeout again.
	lastTimeout *hotstuff.TimeoutMsg

	timer oneShotTimer

	// bag of collected timeout messages for different views
	timeouts *timeoutCollector
}

// New creates a new Synchronizer.
func New(
	// core dependencies
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,

	// security dependencies
	auth *cert.Authority,

	// protocol dependencies
	leaderRotation leaderrotation.LeaderRotation,
	viewDuration ViewDuration,
	timeoutRules TimeoutRuler,
	proposer *consensus.Proposer,
	voter *consensus.Voter,
	state *protocol.ViewStates,

	// network dependencies
	sender core.Sender,
) *Synchronizer {
	s := &Synchronizer{
		leaderRotation: leaderRotation,
		duration:       viewDuration,
		timeoutRules:   timeoutRules,

		proposer:  proposer,
		auth:      auth,
		sender:    sender,
		eventLoop: eventLoop,
		logger:    logger,
		config:    config,
		voter:     voter,
		state:     state,

		timer:    oneShotTimer{time.AfterFunc(0, func() {})}, // dummy timer that will be replaced after start() is called
		timeouts: newTimeoutCollector(config),
	}
	s.eventLoop.RegisterHandler(hotstuff.TimeoutEvent{}, func(event any) {
		timeoutView := event.(hotstuff.TimeoutEvent).View
		if s.state.View() == timeoutView {
			s.OnLocalTimeout()
		}
	})
	s.eventLoop.RegisterHandler(hotstuff.NewViewMsg{}, func(event any) {
		newViewMsg := event.(hotstuff.NewViewMsg)
		s.OnNewView(newViewMsg)
	})
	s.eventLoop.RegisterHandler(hotstuff.TimeoutMsg{}, func(event any) {
		timeoutMsg := event.(hotstuff.TimeoutMsg)
		s.OnRemoteTimeout(timeoutMsg)
	})
	s.eventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		proposal := event.(hotstuff.ProposeMsg)
		// verify the incoming proposal before attempting to vote and try to commit.
		if err := s.voter.Verify(&proposal); err != nil {
			s.logger.Infof("failed to verify incoming vote: %v", err)
			return
		}
		s.logger.Debugf("Received proposal: %v", proposal.Block)
		err := s.voter.OnValidPropose(&proposal)
		if err != nil {
			s.logger.Info(err)
		}
		// advance the view regardless of vote status
		s.advanceView(hotstuff.NewSyncInfo().WithQC(proposal.Block.QuorumCert()))
	})
	return s
}

// A oneShotTimer is a timer that should not be reused.
type oneShotTimer struct {
	timerDoNotUse *time.Timer
}

func (t oneShotTimer) Stop() bool {
	return t.timerDoNotUse.Stop()
}

func (s *Synchronizer) startTimeoutTimer() {
	// Store the view in a local variable to avoid calling s.View() in the closure below,
	// thus avoiding a data race.
	view := s.state.View()
	d := s.duration.Duration()
	// It is important that the timer is NOT reused because then the view would be wrong.
	s.timer = oneShotTimer{time.AfterFunc(d, func() {
		// The event loop will execute onLocalTimeout for us.
		s.eventLoop.AddEvent(hotstuff.TimeoutEvent{View: view})
	})}
}

func (s *Synchronizer) stopTimeoutTimer() {
	s.timer.Stop()
}

// Start starts the synchronizer with the given context.
func (s *Synchronizer) Start(ctx context.Context) {
	s.startTimeoutTimer()

	go func() {
		<-ctx.Done()
		s.stopTimeoutTimer()
	}()

	// start the initial proposal
	if view := s.state.View(); view == 1 && s.leaderRotation.GetLeader(view) == s.config.ID() {
		syncInfo := s.state.SyncInfo()
		proposal, err := s.proposer.CreateProposal(syncInfo)
		if err != nil {
			// debug log here since it may frequently fail due to lack of commands.
			s.logger.Debugf("Failed to create proposal: %v", err)
			return
		}
		s.logger.Debug("Propose")
		if err := s.proposer.Propose(&proposal); err != nil {
			s.logger.Info(err)
			return
		}
	}
}

// OnLocalTimeout is called when a local timeout happens.
func (s *Synchronizer) OnLocalTimeout() {
	s.logger.Debug("OnLocalTimeout")
	s.startTimeoutTimer()
	view := s.state.View()
	if s.lastTimeout != nil && s.lastTimeout.View == view {
		s.sender.Timeout(*s.lastTimeout)
		return
	}
	s.duration.ViewTimeout() // increase the duration of the next view
	s.logger.Debugf("OnLocalTimeout: %v", view)

	timeoutMsg, err := s.timeoutRules.LocalTimeoutRule(view, s.state.SyncInfo())
	if err != nil {
		s.logger.Warnf("Failed to create timeout message: %v", err)
		return
	}
	s.lastTimeout = timeoutMsg
	// stop voting for current view
	prev := s.voter.LastVote()
	s.voter.StopVoting(view)
	// check if view is the same to log vote stop
	if prev != view {
		s.logger.Debugf("Stopped voting in view %d and changed to view %d", prev, view)
	}

	s.sender.Timeout(*timeoutMsg)
	s.OnRemoteTimeout(*timeoutMsg)
}

// OnRemoteTimeout handles an incoming timeout from a remote replica.
func (s *Synchronizer) OnRemoteTimeout(timeout hotstuff.TimeoutMsg) {
	currView := s.state.View()
	defer s.timeouts.deleteOldViews(currView)

	if err := s.auth.Verify(timeout.ViewSignature, timeout.View.ToBytes()); err != nil {
		s.logger.Infof("View timeout signature could not be verified: %v", err)
		return
	}
	s.logger.Debug("OnRemoteTimeout (advancing view): ", timeout)
	s.advanceView(timeout.SyncInfo)

	timeoutList, quorum := s.timeouts.add(timeout)
	if !quorum {
		s.logger.Debugf("OnRemoteTimeout: not enough timeouts for view %d, waiting for more", timeout.View)
		return
	}

	si, err := s.timeoutRules.RemoteTimeoutRule(currView, timeout.View, timeoutList)
	if err != nil {
		// this can only happen if the timeout rule fails to create a quorum certificate
		// or aggregate certificate, e.g., due insufficient number of timeouts.
		s.logger.Debugf("Failed to create sync info: %v", err)
		return
	}
	si = si.WithQC(s.state.HighQC()) // ensure sync info also has the high QC

	s.logger.Debugf("OnRemoteTimeout (second advance)")
	s.advanceView(si)
}

// OnNewView handles an incoming consensus.NewViewMsg
func (s *Synchronizer) OnNewView(newView hotstuff.NewViewMsg) {
	s.logger.Debugf("OnNewView (from network: %t)", newView.FromNetwork)
	if newView.FromNetwork {
		s.logger.Debugf("new view msg from: %d", newView.ID)
	}
	s.advanceView(newView.SyncInfo)
}

// advanceView attempts to advance to the next view using the given QC.
// qc must be either a regular quorum certificate, or a timeout certificate.
func (s *Synchronizer) advanceView(syncInfo hotstuff.SyncInfo) { // nolint: gocyclo
	s.logger.Debugf("advanceView: %v", syncInfo)
	view := hotstuff.View(0)
	timeout := false

	// check for a TC
	if tc, ok := syncInfo.TC(); ok {
		if err := s.auth.VerifyTimeoutCert(tc); err != nil {
			s.logger.Infof("Timeout certificate could not be verified: %v", err)
			return
		}
		s.state.UpdateHighTC(tc)
		view = tc.View()
		timeout = true
	}

	var (
		haveQC bool
		qc     hotstuff.QuorumCert
		aggQC  hotstuff.AggregateQC
	)

	// check for an AggQC or QC
	if aggQC, haveQC = syncInfo.AggQC(); haveQC && s.config.HasAggregateQC() {
		highQC, err := s.auth.VerifyAggregateQC(aggQC)
		if err != nil {
			s.logger.Infof("advanceView: Agg-qc could not be verified: %v", err)
			return
		}
		if aggQC.View() >= view {
			view = aggQC.View()
			timeout = true
		}
		// ensure that the true highQC is the one stored in the syncInfo
		syncInfo = syncInfo.WithQC(highQC)
		qc = highQC
	} else if qc, haveQC = syncInfo.QC(); haveQC {
		if err := s.auth.VerifyQuorumCert(qc); err != nil {
			s.logger.Infof("advanceView: QC could not be verified: %v", err)
			return
		}
	}

	if haveQC {
		oldQC := s.state.HighQC()
		err := s.state.UpdateHighQC(qc)
		if err != nil {
			s.logger.Warnf("advanceView: Failed to update high-qc: %v", err)
		} else {
			newQC := s.state.HighQC()
			if !bytes.Equal(oldQC.ToBytes(), newQC.ToBytes()) {
				s.logger.Debug("advanceView: High-qc updated")
			}
		}
		// if there is both a TC and a QC, we use the QC if its view is greater or equal to the TC.
		if qc.View() >= view {
			view = qc.View()
			timeout = false
		}
	}

	if view < s.state.View() {
		return
	}

	s.stopTimeoutTimer()

	if !timeout {
		s.duration.ViewSucceeded()
	}

	newView := s.state.NextView()

	s.lastTimeout = nil
	s.duration.ViewStarted()

	s.startTimeoutTimer()

	s.logger.Debugf("advanceView: Advanced to view %d", newView)
	s.eventLoop.AddEvent(hotstuff.ViewChangeEvent{View: newView, Timeout: timeout})

	leader := s.leaderRotation.GetLeader(newView)
	if leader == s.config.ID() {
		proposal, err := s.proposer.CreateProposal(syncInfo)
		if err != nil {
			// debug log here since it may frequently fail due to lack of commands.
			s.logger.Debugf("Failed to create proposal: %v", err)
			return
		}
		if err := s.proposer.Propose(&proposal); err != nil {
			s.logger.Info(err)
		}
		return
	}
	err := s.sender.NewView(leader, syncInfo)
	if err != nil {
		s.logger.Warnf("advanceView: error on sending new view: %v", err)
	}
}
