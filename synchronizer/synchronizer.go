package synchronizer

import (
	"context"
	"fmt"
	"time"

	"github.com/relab/hotstuff/modules"

	"github.com/relab/hotstuff"
)

// Synchronizer synchronizes replicas to the same view.
type Synchronizer struct {
	mods *modules.ConsensusCore

	currentView hotstuff.View
	highTC      hotstuff.TimeoutCert
	highQC      hotstuff.QuorumCert
	leafBlock   *hotstuff.Block

	// A pointer to the last timeout message that we sent.
	// If a timeout happens again before we advance to the next view,
	// we will simply send this timeout again.
	lastTimeout *hotstuff.TimeoutMsg

	duration ViewDuration
	timer    *time.Timer

	viewCtx   context.Context // a context that is cancelled at the end of the current view
	cancelCtx context.CancelFunc

	// map of collected timeout messages per view
	timeouts map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg
}

// InitConsensusModule gives the module a reference to the ConsensusCore object.
// It also allows the module to set module options using the OptionsBuilder.
func (s *Synchronizer) InitConsensusModule(mods *modules.ConsensusCore, opts *modules.OptionsBuilder) {
	if duration, ok := s.duration.(modules.Module); ok {
		duration.InitConsensusModule(mods, opts)
	}
	s.mods = mods

	s.mods.EventLoop().RegisterHandler(TimeoutEvent{}, func(event any) {
		timeoutView := event.(TimeoutEvent).View
		if s.currentView == timeoutView {
			s.OnLocalTimeout()
		}
	})

	s.mods.EventLoop().RegisterHandler(hotstuff.NewViewMsg{}, func(event any) {
		newViewMsg := event.(hotstuff.NewViewMsg)
		s.OnNewView(newViewMsg)
	})

	s.mods.EventLoop().RegisterHandler(hotstuff.TimeoutMsg{}, func(event any) {
		timeoutMsg := event.(hotstuff.TimeoutMsg)
		s.OnRemoteTimeout(timeoutMsg)
	})

	var err error
	s.highQC, err = s.mods.Crypto().CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty quorum cert for genesis block: %v", err))
	}
	s.highTC, err = s.mods.Crypto().CreateTimeoutCert(hotstuff.View(0), []hotstuff.TimeoutMsg{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty timeout cert for view 0: %v", err))
	}

}

// New creates a new Synchronizer.
func New(viewDuration ViewDuration) modules.Synchronizer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Synchronizer{
		leafBlock:   hotstuff.GetGenesis(),
		currentView: 1,

		viewCtx:   ctx,
		cancelCtx: cancel,

		duration: viewDuration,
		timer:    time.AfterFunc(0, func() {}), // dummy timer that will be replaced after start() is called

		timeouts: make(map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg),
	}
}

// Start starts the synchronizer with the given context.
func (s *Synchronizer) Start(ctx context.Context) {
	s.timer = time.AfterFunc(s.duration.Duration(), func() {
		// The event loop will execute onLocalTimeout for us.
		s.cancelCtx()
		s.mods.EventLoop().AddEvent(TimeoutEvent{s.currentView})
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
func (s *Synchronizer) HighQC() hotstuff.QuorumCert {
	return s.highQC
}

// LeafBlock returns the current leaf block.
func (s *Synchronizer) LeafBlock() *hotstuff.Block {
	return s.leafBlock
}

// View returns the current view.
func (s *Synchronizer) View() hotstuff.View {
	return s.currentView
}

// ViewContext returns a context that is cancelled at the end of the view.
func (s *Synchronizer) ViewContext() context.Context {
	return s.viewCtx
}

// SyncInfo returns the highest known QC or TC.
func (s *Synchronizer) SyncInfo() hotstuff.SyncInfo {
	return hotstuff.NewSyncInfo().WithQC(s.highQC).WithTC(s.highTC)
}

// OnLocalTimeout is called when a local timeout happens.
func (s *Synchronizer) OnLocalTimeout() {
	// Reset the timer and ctx here so that we can get a new timeout in the same view.
	// I think this is necessary to ensure that we can keep sending the same timeout message
	// until we get a timeout certificate.
	//
	// TODO: figure out the best way to handle this context and timeout.
	if s.viewCtx.Err() != nil {
		s.newCtx(s.duration.Duration())
	}
	s.timer.Reset(s.duration.Duration())

	if s.lastTimeout != nil && s.lastTimeout.View == s.currentView {
		s.mods.Configuration().Timeout(*s.lastTimeout)
		return
	}

	s.duration.ViewTimeout() // increase the duration of the next view
	view := s.currentView
	s.mods.Logger().Debugf("OnLocalTimeout: %v", view)

	sig, err := s.mods.Crypto().Sign(view.ToBytes())
	if err != nil {
		s.mods.Logger().Warnf("Failed to sign view: %v", err)
		return
	}
	timeoutMsg := hotstuff.TimeoutMsg{
		ID:            s.mods.ID(),
		View:          view,
		SyncInfo:      s.SyncInfo(),
		ViewSignature: sig,
	}

	if s.mods.Options().ShouldUseAggQC() {
		// generate a second signature that will become part of the aggregateQC
		sig, err := s.mods.Crypto().Sign(timeoutMsg.ToBytes())
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
func (s *Synchronizer) OnRemoteTimeout(timeout hotstuff.TimeoutMsg) {
	defer func() {
		// cleanup old timeouts
		for view := range s.timeouts {
			if view < s.currentView {
				delete(s.timeouts, view)
			}
		}
	}()

	verifier := s.mods.Crypto()
	if !verifier.Verify(timeout.ViewSignature, timeout.View.ToBytes()) {
		return
	}
	s.mods.Logger().Debug("OnRemoteTimeout: ", timeout)

	s.AdvanceView(timeout.SyncInfo)

	timeouts, ok := s.timeouts[timeout.View]
	if !ok {
		timeouts = make(map[hotstuff.ID]hotstuff.TimeoutMsg)
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
	timeoutList := make([]hotstuff.TimeoutMsg, 0, len(timeouts))
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
		if !s.mods.Crypto().VerifyTimeoutCert(tc) {
			s.mods.Logger().Info("Timeout Certificate could not be verified!")
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
	if aggQC, haveQC = syncInfo.AggQC(); haveQC && s.mods.Options().ShouldUseAggQC() {
		highQC, ok := s.mods.Crypto().VerifyAggregateQC(aggQC)
		if !ok {
			s.mods.Logger().Info("Aggregated Quorum Certificate could not be verified")
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
		if !s.mods.Crypto().VerifyQuorumCert(qc) {
			s.mods.Logger().Info("Quorum Certificate could not be verified!")
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

// updateHighQC attempts to update the highQC, but does not verify the qc first.
// This method is meant to be used instead of the exported UpdateHighQC internally
// in this package when the qc has already been verified.
func (s *Synchronizer) updateHighQC(qc hotstuff.QuorumCert) {
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
func (s *Synchronizer) updateHighTC(tc hotstuff.TimeoutCert) {
	if tc.View() > s.highTC.View() {
		s.highTC = tc
		s.mods.Logger().Debug("HighTC updated")
	}
}

func (s *Synchronizer) newCtx(duration time.Duration) {
	s.cancelCtx()
	s.viewCtx, s.cancelCtx = context.WithTimeout(context.Background(), duration)
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
