// Package synchronizer implements the synchronizer module.
package synchronizer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"

	"github.com/relab/hotstuff"
)

// Synchronizer synchronizes replicas to the same view.
type Synchronizer struct {
	comps core.ComponentList

	leaderRotation modules.LeaderRotation
	duration       modules.ViewDuration

	mut         sync.RWMutex // to protect the following
	currentView hotstuff.View
	highTC      hotstuff.TimeoutCert
	highQC      hotstuff.QuorumCert

	// A pointer to the last timeout message that we sent.
	// If a timeout happens again before we advance to the next view,
	// we will simply send this timeout again.
	lastTimeout *hotstuff.TimeoutMsg

	timer oneShotTimer

	// map of collected timeout messages per view
	timeouts map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg
}

// InitComponent initializes the synchronizer.
func (s *Synchronizer) InitComponent(mods *core.Core) {
	s.comps = mods.Components()

	mods.Get(
		&s.leaderRotation,
		&s.duration,
	)

	s.comps.EventLoop.RegisterHandler(TimeoutEvent{}, func(event any) {
		timeoutView := event.(TimeoutEvent).View
		if s.View() == timeoutView {
			s.OnLocalTimeout()
		}
	})

	s.comps.EventLoop.RegisterHandler(hotstuff.NewViewMsg{}, func(event any) {
		newViewMsg := event.(hotstuff.NewViewMsg)
		s.OnNewView(newViewMsg)
	})

	s.comps.EventLoop.RegisterHandler(hotstuff.TimeoutMsg{}, func(event any) {
		timeoutMsg := event.(hotstuff.TimeoutMsg)
		s.OnRemoteTimeout(timeoutMsg)
	})

	var err error
	s.highQC, err = s.comps.Crypto.CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty quorum cert for genesis block: %v", err))
	}
	s.highTC, err = s.comps.Crypto.CreateTimeoutCert(hotstuff.View(0), []hotstuff.TimeoutMsg{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty timeout cert for view 0: %v", err))
	}
}

// New creates a new Synchronizer.
func New() core.Synchronizer {
	return &Synchronizer{
		currentView: 1,

		timer:    oneShotTimer{time.AfterFunc(0, func() {})}, // dummy timer that will be replaced after start() is called
		timeouts: make(map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg),
	}
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
	view := s.View()
	// It is important that the timer is NOT reused because then the view would be wrong.
	s.timer = oneShotTimer{time.AfterFunc(s.duration.Duration(), func() {
		// The event loop will execute onLocalTimeout for us.
		s.comps.EventLoop.AddEvent(TimeoutEvent{view})
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
	if view := s.View(); view == 1 && s.leaderRotation.GetLeader(view) == s.comps.Options.ID() {
		s.comps.Consensus.Propose(s.SyncInfo())
	}
}

// HighQC returns the highest known QC.
func (s *Synchronizer) HighQC() hotstuff.QuorumCert {
	return s.highQC
}

// View returns the current view.
func (s *Synchronizer) View() hotstuff.View {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.currentView
}

// SyncInfo returns the highest known QC or TC.
func (s *Synchronizer) SyncInfo() hotstuff.SyncInfo {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return hotstuff.NewSyncInfo().WithQC(s.highQC).WithTC(s.highTC)
}

// OnLocalTimeout is called when a local timeout happens.
func (s *Synchronizer) OnLocalTimeout() {
	s.startTimeoutTimer()

	view := s.View()

	if s.lastTimeout != nil && s.lastTimeout.View == view {
		s.comps.Configuration.Timeout(*s.lastTimeout)
		return
	}

	s.duration.ViewTimeout() // increase the duration of the next view
	s.comps.Logger.Debugf("OnLocalTimeout: %v", view)

	sig, err := s.comps.Crypto.Sign(view.ToBytes())
	if err != nil {
		s.comps.Logger.Warnf("Failed to sign view: %v", err)
		return
	}
	timeoutMsg := hotstuff.TimeoutMsg{
		ID:            s.comps.Options.ID(),
		View:          view,
		SyncInfo:      s.SyncInfo(),
		ViewSignature: sig,
	}

	if s.comps.Options.ShouldUseAggQC() {
		// generate a second signature that will become part of the aggregateQC
		sig, err := s.comps.Crypto.Sign(timeoutMsg.ToBytes())
		if err != nil {
			s.comps.Logger.Warnf("Failed to sign timeout message: %v", err)
			return
		}
		timeoutMsg.MsgSignature = sig
	}
	s.lastTimeout = &timeoutMsg
	// stop voting for current view
	s.comps.Consensus.StopVoting(view)

	s.comps.Configuration.Timeout(timeoutMsg)
	s.OnRemoteTimeout(timeoutMsg)
}

// OnRemoteTimeout handles an incoming timeout from a remote replica.
func (s *Synchronizer) OnRemoteTimeout(timeout hotstuff.TimeoutMsg) {
	currView := s.View()

	defer func() {
		// cleanup old timeouts
		for view := range s.timeouts {
			if view < currView {
				delete(s.timeouts, view)
			}
		}
	}()

	verifier := s.comps.Crypto
	if !verifier.Verify(timeout.ViewSignature, timeout.View.ToBytes()) {
		return
	}
	s.comps.Logger.Debug("OnRemoteTimeout: ", timeout)

	s.AdvanceView(timeout.SyncInfo)

	timeouts, ok := s.timeouts[timeout.View]
	if !ok {
		timeouts = make(map[hotstuff.ID]hotstuff.TimeoutMsg)
		s.timeouts[timeout.View] = timeouts
	}

	if _, ok := timeouts[timeout.ID]; !ok {
		timeouts[timeout.ID] = timeout
	}

	if len(timeouts) < s.comps.Configuration.QuorumSize() {
		return
	}

	// TODO: should probably change CreateTimeoutCert and maybe also CreateQuorumCert
	// to use maps instead of slices
	timeoutList := make([]hotstuff.TimeoutMsg, 0, len(timeouts))
	for _, t := range timeouts {
		timeoutList = append(timeoutList, t)
	}

	tc, err := s.comps.Crypto.CreateTimeoutCert(timeout.View, timeoutList)
	if err != nil {
		s.comps.Logger.Debugf("Failed to create timeout certificate: %v", err)
		return
	}

	si := s.SyncInfo().WithTC(tc)

	if s.comps.Options.ShouldUseAggQC() {
		aggQC, err := s.comps.Crypto.CreateAggregateQC(currView, timeoutList)
		if err != nil {
			s.comps.Logger.Debugf("Failed to create aggregateQC: %v", err)
		} else {
			si = si.WithAggQC(aggQC)
		}
	}

	delete(s.timeouts, timeout.View)

	s.AdvanceView(si)
}

// OnNewView handles an incoming comps.Consensus.NewViewMsg
func (s *Synchronizer) OnNewView(newView hotstuff.NewViewMsg) {
	s.AdvanceView(newView.SyncInfo)
}

// AdvanceView attempts to advance to the next view using the given QC.
// qc must be either a regular quorum certificate, or a timeout certificate.
func (s *Synchronizer) AdvanceView(syncInfo hotstuff.SyncInfo) {
	v := hotstuff.View(0)
	timeout := false

	// check for a TC
	if tc, ok := syncInfo.TC(); ok {
		if !s.comps.Crypto.VerifyTimeoutCert(tc) {
			s.comps.Logger.Info("Timeout Certificate could not be verified!")
			return
		}
		s.updateHighTC(tc)
		v = tc.View()
		timeout = true
	}

	var (
		haveQC bool
		qc     hotstuff.QuorumCert
		aggQC  hotstuff.AggregateQC
	)

	// check for an AggQC or QC
	if aggQC, haveQC = syncInfo.AggQC(); haveQC && s.comps.Options.ShouldUseAggQC() {
		highQC, ok := s.comps.Crypto.VerifyAggregateQC(aggQC)
		if !ok {
			s.comps.Logger.Info("Aggregated Quorum Certificate could not be verified")
			return
		}
		if aggQC.View() >= v {
			v = aggQC.View()
			timeout = true
		}
		// ensure that the true highQC is the one stored in the syncInfo
		syncInfo = syncInfo.WithQC(highQC)
		qc = highQC
	} else if qc, haveQC = syncInfo.QC(); haveQC {
		if !s.comps.Crypto.VerifyQuorumCert(qc) {
			s.comps.Logger.Info("Quorum Certificate could not be verified!")
			return
		}
	}

	if haveQC {
		s.updateHighQC(qc)
		// if there is both a TC and a QC, we use the QC if its view is greater or equal to the TC.
		if qc.View() >= v {
			v = qc.View()
			timeout = false
		}
	}

	if v < s.View() {
		return
	}

	s.stopTimeoutTimer()

	if !timeout {
		s.duration.ViewSucceeded()
	}

	newView := v + 1

	s.currentView = newView

	s.lastTimeout = nil
	s.duration.ViewStarted()

	s.startTimeoutTimer()

	s.comps.Logger.Debugf("advanced to view %d", newView)
	s.comps.EventLoop.AddEvent(ViewChangeEvent{View: newView, Timeout: timeout})

	leader := s.leaderRotation.GetLeader(newView)
	if leader == s.comps.Options.ID() {
		s.comps.Consensus.Propose(syncInfo)
	} else if replica, ok := s.comps.Configuration.Replica(leader); ok {
		replica.NewView(syncInfo)
	}
}

// updateHighQC attempts to update the highQC, but does not verify the qc first.
// This method is meant to be used instead of the exported UpdateHighQC internally
// in this package when the qc has already been verified.
func (s *Synchronizer) updateHighQC(qc hotstuff.QuorumCert) {
	newBlock, ok := s.comps.BlockChain.Get(qc.BlockHash())
	if !ok {
		s.comps.Logger.Info("updateHighQC: Could not find block referenced by new QC!")
		return
	}

	if newBlock.View() > s.highQC.View() {
		s.highQC = qc
		s.comps.Logger.Debug("HighQC updated")
	}
}

// updateHighTC attempts to update the highTC, but does not verify the tc first.
func (s *Synchronizer) updateHighTC(tc hotstuff.TimeoutCert) {
	if tc.View() > s.highTC.View() {
		s.highTC = tc
		s.comps.Logger.Debug("HighTC updated")
	}
}

var _ core.Synchronizer = (*Synchronizer)(nil)

// ViewChangeEvent is sent on the eventloop whenever a view change occurs.
type ViewChangeEvent struct {
	View    hotstuff.View
	Timeout bool
}

// TimeoutEvent is sent on the eventloop when a local timeout occurs.
type TimeoutEvent struct {
	View hotstuff.View
}
