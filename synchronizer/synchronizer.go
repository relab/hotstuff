package synchronizer

import (
	"context"
	"fmt"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
)

// Synchronizer synchronizes replicas to the same view.
type Synchronizer struct {
	mods *consensus.Modules

	currentView consensus.View
	highTC      consensus.TimeoutCert
	highQC      consensus.QuorumCert
	leafBlock   *consensus.Block

	// A pointer to the last timeout message that we sent.
	// If a timeout happens again before we advance to the next view,
	// we will simply send this timeout again.
	lastTimeout *consensus.TimeoutMsg

	duration ViewDuration
	timer    *time.Timer

	viewCtx   context.Context // a context that is cancelled at the end of the current view
	cancelCtx context.CancelFunc

	// map of collected timeout messages per view
	timeouts map[consensus.View]map[hotstuff.ID]consensus.TimeoutMsg
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (s *Synchronizer) InitConsensusModule(mods *consensus.Modules, opts *consensus.OptionsBuilder) {
	if duration, ok := s.duration.(consensus.Module); ok {
		duration.InitConsensusModule(mods, opts)
	}
	s.mods = mods

	s.mods.EventLoop().RegisterHandler(consensus.NewViewMsg{}, func(event interface{}) {
		newViewMsg := event.(consensus.NewViewMsg)
		s.OnNewView(newViewMsg)
	})

	s.mods.EventLoop().RegisterHandler(consensus.TimeoutMsg{}, func(event interface{}) {
		timeoutMsg := event.(consensus.TimeoutMsg)
		s.OnRemoteTimeout(timeoutMsg)
	})

	var err error
	s.highQC, err = s.mods.Crypto().CreateQuorumCert(consensus.GetGenesis(), []consensus.PartialCert{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty quorum cert for genesis block: %v", err))
	}
	s.highTC, err = s.mods.Crypto().CreateTimeoutCert(consensus.View(0), []consensus.TimeoutMsg{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty timeout cert for view 0: %v", err))
	}

}

// New creates a new Synchronizer.
func New(viewDuration ViewDuration) consensus.Synchronizer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Synchronizer{
		leafBlock:   consensus.GetGenesis(),
		currentView: 1,

		viewCtx:   ctx,
		cancelCtx: cancel,

		duration: viewDuration,
		timer:    time.AfterFunc(0, func() {}), // dummy timer that will be replaced after start() is called

		timeouts: make(map[consensus.View]map[hotstuff.ID]consensus.TimeoutMsg),
	}
}

// Start starts the synchronizer with the given context.
func (s *Synchronizer) Start(ctx context.Context) {
	s.timer = time.AfterFunc(s.duration.Duration(), func() {
		// The event loop will execute onLocalTimeout for us.
		s.cancelCtx()
		s.mods.EventLoop().AddEvent(s.OnLocalTimeout)
	})

	go func() {
		<-ctx.Done()
		s.timer.Stop()
	}()

	// start the initial proposal
	if s.currentView == 1 && s.mods.LeaderRotation().GetLeader(s.currentView) == s.mods.ID() {
		s.mods.Consensus().Propose(s.SyncInfo())
	}
}

// HighQC returns the highest known QC.
func (s *Synchronizer) HighQC() consensus.QuorumCert {
	return s.highQC
}

// LeafBlock returns the current leaf block.
func (s *Synchronizer) LeafBlock() *consensus.Block {
	return s.leafBlock
}

// View returns the current view.
func (s *Synchronizer) View() consensus.View {
	return s.currentView
}

// ViewContext returns a context that is cancelled at the end of the view.
func (s *Synchronizer) ViewContext() context.Context {
	return s.viewCtx
}

// SyncInfo returns the highest known QC or TC.
func (s *Synchronizer) SyncInfo() consensus.SyncInfo {
	if s.highQC.View() >= s.highTC.View() {
		return consensus.NewSyncInfo().WithQC(s.highQC)
	}
	return consensus.NewSyncInfo().WithQC(s.highQC).WithTC(s.highTC)
}

// OnLocalTimeout is called when a local timeout happens.
func (s *Synchronizer) OnLocalTimeout() {
	defer func() {
		// Reset the timer and ctx here so that we can get a new timeout in the same view.
		// I think this is necessary to ensure that we can keep sending the same timeout message
		// until we get a timeout certificate.
		//
		// TODO: figure out the best way to handle this context and timeout.
		if s.viewCtx.Err() != nil {
			s.newCtx(s.duration.Duration())
		}
		s.timer.Reset(s.duration.Duration())
	}()

	if s.lastTimeout != nil && s.lastTimeout.View == s.currentView {
		s.mods.Configuration().Timeout(*s.lastTimeout)
		return
	}

	s.duration.ViewTimeout() // increase the duration of the next view
	view := s.currentView
	s.mods.Logger().Debugf("OnLocalTimeout: %v", view)

	sig, err := s.mods.Crypto().Sign(view.ToHash())
	if err != nil {
		s.mods.Logger().Warnf("Failed to sign view: %v", err)
		return
	}
	timeoutMsg := consensus.TimeoutMsg{
		ID:            s.mods.ID(),
		View:          view,
		SyncInfo:      s.SyncInfo(),
		ViewSignature: sig,
	}

	if s.mods.Options().ShouldUseAggQC() {
		// generate a second signature that will become part of the aggregateQC
		sig, err := s.mods.Crypto().Sign(timeoutMsg.Hash())
		if err != nil {
			s.mods.Logger().Warnf("Failed to sign timeout message: %v", err)
			return
		}
		timeoutMsg.MsgSignature = sig
	}
	s.lastTimeout = &timeoutMsg
	// stop voting for current view
	s.mods.Consensus().StopVoting(s.currentView)

	s.mods.Configuration().Timeout(timeoutMsg)
	s.OnRemoteTimeout(timeoutMsg)
}

// OnRemoteTimeout handles an incoming timeout from a remote replica.
func (s *Synchronizer) OnRemoteTimeout(timeout consensus.TimeoutMsg) {
	defer func() {
		// cleanup old timeouts
		for view := range s.timeouts {
			if view < s.currentView {
				delete(s.timeouts, view)
			}
		}
	}()

	verifier := s.mods.Crypto()
	if !verifier.Verify(timeout.ViewSignature, timeout.View.ToHash()) {
		return
	}
	s.mods.Logger().Debug("OnRemoteTimeout: ", timeout)

	s.AdvanceView(timeout.SyncInfo)

	timeouts, ok := s.timeouts[timeout.View]
	if !ok {
		timeouts = make(map[hotstuff.ID]consensus.TimeoutMsg)
		s.timeouts[timeout.View] = timeouts
	}

	if _, ok := timeouts[timeout.ID]; !ok {
		timeouts[timeout.ID] = timeout
	}

	if len(timeouts) < s.mods.Configuration().QuorumSize() {
		return
	}

	// TODO: should probably change CreateTimeoutCert and maybe also CreateQuorumCert
	// to use maps instead of slices
	timeoutList := make([]consensus.TimeoutMsg, 0, len(timeouts))
	for _, t := range timeouts {
		timeoutList = append(timeoutList, t)
	}

	tc, err := s.mods.Crypto().CreateTimeoutCert(timeout.View, timeoutList)
	if err != nil {
		s.mods.Logger().Debugf("Failed to create timeout certificate: %v", err)
		return
	}

	si := s.SyncInfo().WithTC(tc)

	if s.mods.Options().ShouldUseAggQC() {
		aggQC, err := s.mods.Crypto().CreateAggregateQC(s.currentView, timeoutList)
		if err != nil {
			s.mods.Logger().Debugf("Failed to create aggregateQC: %v", err)
		} else {
			si = si.WithAggQC(aggQC)
		}
	}

	delete(s.timeouts, timeout.View)

	s.AdvanceView(si)
}

// OnNewView handles an incoming consensus.NewViewMsg
func (s *Synchronizer) OnNewView(newView consensus.NewViewMsg) {
	s.AdvanceView(newView.SyncInfo)
}

// AdvanceView attempts to advance to the next view using the given QC.
// qc must be either a regular quorum certificate, or a timeout certificate.
func (s *Synchronizer) AdvanceView(syncInfo consensus.SyncInfo) {
	v := consensus.View(0)
	timeout := false

	// check for a TC
	if tc, ok := syncInfo.TC(); ok {
		if !s.mods.Crypto().VerifyTimeoutCert(tc) {
			s.mods.Logger().Info("Timeout Certificate could not be verified!")
			return
		}
		s.updateHighTC(tc)
		v = tc.View()
		timeout = true
	}

	// check for a QC.
	if qc, ok := syncInfo.QC(); ok {
		if !s.mods.Crypto().VerifyQuorumCert(qc) {
			s.mods.Logger().Info("Quorum Certificate could not be verified!")
			return
		}
		s.updateHighQC(qc)
		// if there is both a TC and a QC, we use the QC if its view is greater or equal to the TC.
		if qc.View() >= v {
			v = qc.View()
			timeout = false
		}
	}

	if v < s.currentView {
		return
	}

	s.timer.Stop()

	if !timeout {
		s.duration.ViewSucceeded()
	}

	s.currentView = v + 1
	s.lastTimeout = nil
	s.duration.ViewStarted()

	duration := s.duration.Duration()
	// cancel the old view context and set up the next one
	s.newCtx(duration)
	s.timer.Reset(duration)

	s.mods.Logger().Debugf("advanced to view %d", s.currentView)
	s.mods.EventLoop().AddEvent(ViewChangeEvent{View: s.currentView, Timeout: timeout})

	leader := s.mods.LeaderRotation().GetLeader(s.currentView)
	if leader == s.mods.ID() {
		s.mods.Consensus().Propose(syncInfo)
	} else if replica, ok := s.mods.Configuration().Replica(leader); ok {
		replica.NewView(syncInfo)
	}
}

// UpdateHighQC updates HighQC if the given qc is higher than the old HighQC.
func (s *Synchronizer) UpdateHighQC(qc consensus.QuorumCert) {
	s.mods.Logger().Debugf("updateHighQC: %v", qc)
	if !s.mods.Crypto().VerifyQuorumCert(qc) {
		s.mods.Logger().Info("updateHighQC: QC could not be verified!")
		return
	}

	s.updateHighQC(qc)
}

// updateHighQC attempts to update the highQC, but does not verify the qc first.
// This method is meant to be used instead of the exported UpdateHighQC internally
// in this package when the qc has already been verified.
func (s *Synchronizer) updateHighQC(qc consensus.QuorumCert) {
	newBlock, ok := s.mods.BlockChain().Get(qc.BlockHash())
	if !ok {
		s.mods.Logger().Info("updateHighQC: Could not find block referenced by new QC!")
		return
	}

	oldBlock, ok := s.mods.BlockChain().Get(s.highQC.BlockHash())
	if !ok {
		s.mods.Logger().Panic("Block from the old highQC missing from chain")
	}

	if newBlock.View() > oldBlock.View() {
		s.highQC = qc
		s.leafBlock = newBlock
		s.mods.Logger().Debug("HighQC updated")
	}
}

// updateHighTC attempts to update the highTC, but does not verify the tc first.
func (s *Synchronizer) updateHighTC(tc consensus.TimeoutCert) {
	if tc.View() > s.highTC.View() {
		s.highTC = tc
		s.mods.Logger().Debug("HighTC updated")
	}
}

func (s *Synchronizer) newCtx(duration time.Duration) {
	s.cancelCtx()
	s.viewCtx, s.cancelCtx = context.WithTimeout(context.Background(), duration)
}

var _ consensus.Synchronizer = (*Synchronizer)(nil)

// ViewChangeEvent is sent on the metrics event loop whenever a view change occurs.
type ViewChangeEvent struct {
	View    consensus.View
	Timeout bool
}
