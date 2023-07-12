// Package synchronizer implements the synchronizer module.
package synchronizer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"

	"github.com/relab/hotstuff"
)

// Synchronizer synchronizes replicas to the same view.
type Synchronizer struct {
	blockChain     modules.BlockChain
	consensus      modules.Consensus
	crypto         modules.Crypto
	configuration  modules.Configuration
	eventLoop      *eventloop.EventLoop
	leaderRotation modules.LeaderRotation
	logger         logging.Logger
	opts           *modules.Options
	acceptor       modules.Acceptor
	mut            sync.RWMutex // to protect the following
	currentView    hotstuff.View
	highTC         hotstuff.TimeoutCert
	highQC         hotstuff.QuorumCert

	// A pointer to the last timeout message that we sent.
	// If a timeout happens again before we advance to the next view,
	// we will simply send this timeout again.
	lastTimeout *hotstuff.TimeoutMsg

	duration ViewDuration
	timer    *time.Timer

	// map of collected timeout messages per view
	timeouts map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg
}

// InitModule initializes the synchronizer.
func (s *Synchronizer) InitModule(mods *modules.Core) {
	mods.Get(
		&s.blockChain,
		&s.consensus,
		&s.crypto,
		&s.configuration,
		&s.eventLoop,
		&s.leaderRotation,
		&s.logger,
		&s.opts,
		&s.acceptor,
	)

	s.eventLoop.RegisterHandler(TimeoutEvent{}, func(event any) {
		timeoutView := event.(TimeoutEvent).View
		if s.View() == timeoutView {
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

	s.eventLoop.RegisterHandler(hotstuff.Update{}, func(event any) {
		update := event.(hotstuff.Update)
		s.handleUpdateEvent(update.Block, update.QuorumSize)
	})

	var err error
	s.highQC, err = s.crypto.CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty quorum cert for genesis block: %v", err))
	}
	s.highTC, err = s.crypto.CreateTimeoutCert(hotstuff.View(0), []hotstuff.TimeoutMsg{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty timeout cert for view 0: %v", err))
	}
}

// New creates a new Synchronizer.
func New(viewDuration ViewDuration) modules.Synchronizer {
	return &Synchronizer{
		currentView: 1,

		duration: viewDuration,
		timer:    time.AfterFunc(0, func() {}), // dummy timer that will be replaced after start() is called

		timeouts: make(map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg),
	}
}

func (s *Synchronizer) startTimeoutTimer() {
	view := s.View()
	s.timer = time.AfterFunc(s.duration.Duration(), func() {
		// The event loop will execute onLocalTimeout for us.
		s.eventLoop.AddEvent(TimeoutEvent{view})
	})
}

// Start starts the synchronizer with the given context.
func (s *Synchronizer) Start(ctx context.Context) {
	s.startTimeoutTimer()

	go func() {
		<-ctx.Done()
		s.timer.Stop()
	}()

	// start the initial proposal
	if view := s.View(); view == 1 && s.leaderRotation.GetLeader(view) == s.opts.ID() {
		s.consensus.Propose(s.SyncInfo())
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
		s.configuration.Timeout(*s.lastTimeout)
		return
	}

	s.duration.ViewTimeout() // increase the duration of the next view
	s.logger.Debugf("OnLocalTimeout: %v", view)

	sig, err := s.crypto.Sign(view.ToBytes())
	if err != nil {
		s.logger.Warnf("Failed to sign view: %v", err)
		return
	}
	timeoutMsg := hotstuff.TimeoutMsg{
		ID:            s.opts.ID(),
		View:          view,
		SyncInfo:      s.SyncInfo(),
		ViewSignature: sig,
	}

	if s.opts.ShouldUseAggQC() {
		// generate a second signature that will become part of the aggregateQC
		sig, err := s.crypto.Sign(timeoutMsg.ToBytes())
		if err != nil {
			s.logger.Warnf("Failed to sign timeout message: %v", err)
			return
		}
		timeoutMsg.MsgSignature = sig
	}
	s.lastTimeout = &timeoutMsg
	// stop voting for current view
	s.consensus.StopVoting(view)

	s.configuration.Timeout(timeoutMsg)
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

	verifier := s.crypto
	if !verifier.Verify(timeout.ViewSignature, timeout.View.ToBytes()) {
		return
	}
	s.logger.Debug("OnRemoteTimeout: ", timeout)

	s.AdvanceView(timeout.SyncInfo, false)

	timeouts, ok := s.timeouts[timeout.View]
	if !ok {
		timeouts = make(map[hotstuff.ID]hotstuff.TimeoutMsg)
		s.timeouts[timeout.View] = timeouts
	}

	if _, ok := timeouts[timeout.ID]; !ok {
		timeouts[timeout.ID] = timeout
	}

	if len(timeouts) < s.configuration.QuorumSize(timeout.View) {
		return
	}

	// TODO: should probably change CreateTimeoutCert and maybe also CreateQuorumCert
	// to use maps instead of slices
	timeoutList := make([]hotstuff.TimeoutMsg, 0, len(timeouts))
	for _, t := range timeouts {
		timeoutList = append(timeoutList, t)
	}

	tc, err := s.crypto.CreateTimeoutCert(timeout.View, timeoutList)
	if err != nil {
		s.logger.Debugf("Failed to create timeout certificate: %v", err)
		return
	}

	si := s.SyncInfo().WithTC(tc)

	if s.opts.ShouldUseAggQC() {
		aggQC, err := s.crypto.CreateAggregateQC(currView, timeoutList)
		if err != nil {
			s.logger.Debugf("Failed to create aggregateQC: %v", err)
		} else {
			si = si.WithAggQC(aggQC)
		}
	}

	delete(s.timeouts, timeout.View)

	s.AdvanceView(si, false)
}

// OnNewView handles an incoming consensus.NewViewMsg
func (s *Synchronizer) OnNewView(newView hotstuff.NewViewMsg) {
	s.AdvanceView(newView.SyncInfo, false)
}

// AdvanceView attempts to advance to the next view using the given QC.
// qc must be either a regular quorum certificate, or a timeout certificate.
func (s *Synchronizer) AdvanceView(syncInfo hotstuff.SyncInfo, isReconfig bool) {
	v := hotstuff.View(0)
	timeout := false

	// check for a TC
	if tc, ok := syncInfo.TC(); ok {
		if !s.crypto.VerifyTimeoutCert(tc) {
			s.logger.Info("Timeout Certificate could not be verified!")
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
	if aggQC, haveQC = syncInfo.AggQC(); haveQC && s.opts.ShouldUseAggQC() {
		highQC, ok := s.crypto.VerifyAggregateQC(aggQC)
		if !ok {
			s.logger.Info("Aggregated Quorum Certificate could not be verified")
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
		if !s.crypto.VerifyQuorumCert(qc) {
			s.logger.Info("Quorum Certificate could not be verified!")
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

	if v < s.View() && !isReconfig {
		return
	}

	s.timer.Stop()

	if !timeout {
		s.duration.ViewSucceeded()
	}

	newView := v + 1

	s.lastTimeout = nil
	s.duration.ViewStarted()

	duration := s.duration.Duration()
	if isReconfig {
		s.currentView = newView + 1
		s.timer.Reset(s.duration.StartTimeout())
	} else {
		s.currentView = newView
		s.timer.Reset(duration)
	}
	s.eventLoop.AddEvent(ViewChangeEvent{View: newView, Timeout: timeout})
	leader := s.leaderRotation.GetLeader(newView)
	s.logger.Info(" advance to view ", s.currentView, leader)
	if leader == s.opts.ID() {
		s.consensus.Propose(syncInfo)
	} else if replica, ok := s.configuration.Replica(leader); ok {
		replica.NewView(syncInfo)
	}
}

// updateHighQC attempts to update the highQC, but does not verify the qc first.
// This method is meant to be used instead of the exported UpdateHighQC internally
// in this package when the qc has already been verified.
func (s *Synchronizer) updateHighQC(qc hotstuff.QuorumCert) {
	newBlock, ok := s.blockChain.Get(qc.BlockHash())
	if !ok {
		s.logger.Info("updateHighQC: Could not find block referenced by new QC!")
		return
	}

	if newBlock.View() > s.highQC.View() {
		s.highQC = qc
		s.logger.Debug("HighQC updated")
	}
}

// updateHighTC attempts to update the highTC, but does not verify the tc first.
func (s *Synchronizer) updateHighTC(tc hotstuff.TimeoutCert) {
	if tc.View() > s.highTC.View() {
		s.highTC = tc
		s.logger.Debug("HighTC updated")
	}
}

func (s *Synchronizer) Pause(qc hotstuff.QuorumCert) {
	s.timer.Stop()
	s.performCheckPoint(qc)
}

func (s *Synchronizer) Resume(qc hotstuff.QuorumCert) {
	s.performCheckPoint(qc)
	s.timer = time.AfterFunc(s.duration.Duration(), func() {
		// The event loop will execute onLocalTimeout for us.
		s.eventLoop.AddEvent(TimeoutEvent{View: s.currentView})
	})
	s.AdvanceView(hotstuff.NewSyncInfo().WithQC(qc), true)
}

func (s *Synchronizer) performCheckPoint(qc hotstuff.QuorumCert) {
	// Check if the latest qc received from the reconfiguration request
	// is higher than the qc we have seen.

	currentHighQCView := s.highQC.View()

	if currentHighQCView < s.highTC.View() {
		currentHighQCView = s.highTC.View()
	}
	for currentHighQCView < qc.View()-1 {
		block, ok := s.blockChain.Get(qc.BlockHash())
		if !ok {
			break
		}
		s.acceptor.Proposed(block.Command())
		s.blockChain.Store(block)
		currentHighQCView = block.QuorumCert().View()
	}
	// This should repeatedly committ the inner blocks.
	block, ok := s.blockChain.Get(qc.BlockHash())
	if ok {
		s.consensus.Commit(block)
	}
	s.logger.Info("checkpoint completed", currentHighQCView, qc.View())
}

var _ modules.Synchronizer = (*Synchronizer)(nil)

// ViewChangeEvent is sent on the eventloop whenever a view change occurs.
type ViewChangeEvent struct {
	View    hotstuff.View
	Timeout bool
}

// TimeoutEvent is sent on the eventloop when a local timeout occurs.
type TimeoutEvent struct {
	View hotstuff.View
}

func (s *Synchronizer) handleUpdateEvent(block *hotstuff.Block, quorumSize int) {
	if !s.crypto.VerifyQuorumCert(block.QuorumCert()) {
		return
	}
	s.blockChain.Store(block)
	s.updateHighQC(block.QuorumCert())
	s.currentView = block.QuorumCert().View() + 1
	s.consensus.Commit(block)
}
