package synchronizer

import (
	"context"
	"fmt"
	"time"

	"github.com/relab/hotstuff/consensus"
)

// Synchronizer synchronizes replicas to the same view.
type Synchronizer struct {
	mod *consensus.Modules

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
	timeouts map[consensus.View]map[consensus.ID]consensus.TimeoutMsg
}

// InitModule initializes the synchronizer with the given HotStuff instance.
func (s *Synchronizer) InitModule(hs *consensus.Modules, opts *consensus.OptionsBuilder) {
	if duration, ok := s.duration.(consensus.Module); ok {
		duration.InitModule(hs, opts)
	}
	s.mod = hs

	s.mod.EventLoop().RegisterHandler(func(event interface{}) (consume bool) {
		newViewMsg := event.(consensus.NewViewMsg)
		s.OnNewView(newViewMsg)
		return true
	}, consensus.NewViewMsg{})

	s.mod.EventLoop().RegisterHandler(func(event interface{}) (consume bool) {
		timeoutMsg := event.(consensus.TimeoutMsg)
		s.OnRemoteTimeout(timeoutMsg)
		return true
	}, consensus.TimeoutMsg{})

	var err error
	s.highQC, err = s.mod.Crypto().CreateQuorumCert(consensus.GetGenesis(), []consensus.PartialCert{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty quorum cert for genesis block: %v", err))
	}
	s.highTC, err = s.mod.Crypto().CreateTimeoutCert(consensus.View(0), []consensus.TimeoutMsg{})
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

		timeouts: make(map[consensus.View]map[consensus.ID]consensus.TimeoutMsg),
	}
}

// Start starts the synchronizer with the given context.
func (s *Synchronizer) Start(ctx context.Context) {
	s.timer = time.AfterFunc(s.duration.Duration(), func() {
		// The event loop will execute onLocalTimeout for us.
		s.cancelCtx()
		s.mod.EventLoop().AddEvent(s.onLocalTimeout)
	})

	go func() {
		<-ctx.Done()
		s.timer.Stop()
	}()
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

func (s *Synchronizer) onLocalTimeout() {
	defer func() {
		s.cancelCtx()
		s.viewCtx, s.cancelCtx = context.WithCancel(context.Background())
		s.timer.Reset(s.duration.Duration())
	}()

	if s.lastTimeout != nil && s.lastTimeout.View == s.currentView {
		s.mod.Configuration().Timeout(*s.lastTimeout)
		return
	}

	s.duration.ViewTimeout() // increase the duration of the next view
	view := s.currentView
	s.mod.Logger().Debugf("OnLocalTimeout: %v", view)

	sig, err := s.mod.Crypto().Sign(view.ToHash())
	if err != nil {
		s.mod.Logger().Warnf("Failed to sign view: %v", err)
		return
	}
	timeoutMsg := consensus.TimeoutMsg{
		ID:            s.mod.ID(),
		View:          view,
		SyncInfo:      s.SyncInfo(),
		ViewSignature: sig,
	}

	if s.mod.Options().ShouldUseAggQC() {
		// generate a second signature that will become part of the aggregateQC
		sig, err := s.mod.Crypto().Sign(timeoutMsg.Hash())
		if err != nil {
			s.mod.Logger().Warnf("Failed to sign timeout message: %v", err)
			return
		}
		timeoutMsg.MsgSignature = sig
	}
	s.lastTimeout = &timeoutMsg
	// stop voting for current view
	s.mod.Consensus().StopVoting(s.currentView)

	s.mod.Configuration().Timeout(timeoutMsg)
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

	verifier := s.mod.Crypto()
	if !verifier.Verify(timeout.ViewSignature, timeout.View.ToHash()) {
		return
	}
	s.mod.Logger().Debug("OnRemoteTimeout: ", timeout)

	s.AdvanceView(timeout.SyncInfo)

	timeouts, ok := s.timeouts[timeout.View]
	if !ok {
		timeouts = make(map[consensus.ID]consensus.TimeoutMsg)
		s.timeouts[timeout.View] = timeouts
	}

	if _, ok := timeouts[timeout.ID]; !ok {
		timeouts[timeout.ID] = timeout
	}

	if len(timeouts) < s.mod.Configuration().QuorumSize() {
		return
	}

	// TODO: should probably change CreateTimeoutCert and maybe also CreateQuorumCert
	// to use maps instead of slices
	timeoutList := make([]consensus.TimeoutMsg, 0, len(timeouts))
	for _, t := range timeouts {
		timeoutList = append(timeoutList, t)
	}

	tc, err := s.mod.Crypto().CreateTimeoutCert(timeout.View, timeoutList)
	if err != nil {
		s.mod.Logger().Debugf("Failed to create timeout certificate: %v", err)
		return
	}

	si := s.SyncInfo().WithTC(tc)

	if s.mod.Options().ShouldUseAggQC() {
		aggQC, err := s.mod.Crypto().CreateAggregateQC(s.currentView, timeoutList)
		if err != nil {
			s.mod.Logger().Debugf("Failed to create aggregateQC: %v", err)
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
	var v consensus.View
	if tc, ok := syncInfo.TC(); ok {
		if !s.mod.Crypto().VerifyTimeoutCert(tc) {
			s.mod.Logger().Info("Timeout Certificate could not be verified!")
			return
		}
		v = tc.View()
		if v > s.highTC.View() {
			s.highTC = tc
		}
	} else if qc, ok := syncInfo.QC(); ok {
		if !s.mod.Crypto().VerifyQuorumCert(qc) {
			s.mod.Logger().Info("Quorum Certificate could not be verified!")
			return
		}
		s.UpdateHighQC(qc)
		v = qc.View()
		s.duration.ViewSucceeded()
	}

	if v < s.currentView {
		return
	}

	s.currentView = v + 1
	s.lastTimeout = nil
	s.duration.ViewStarted()

	// cancel the old view context and set up the next one
	s.cancelCtx()
	s.viewCtx, s.cancelCtx = context.WithCancel(context.Background())

	leader := s.mod.LeaderRotation().GetLeader(s.currentView)
	if leader == s.mod.ID() {
		s.mod.Consensus().Propose(syncInfo)
	} else if replica, ok := s.mod.Configuration().Replica(leader); ok {
		replica.NewView(syncInfo)
	}
}

// UpdateHighQC updates HighQC if the given qc is higher than the old HighQC.
func (s *Synchronizer) UpdateHighQC(qc consensus.QuorumCert) {
	s.mod.Logger().Debugf("updateHighQC: %v", qc)
	if !s.mod.Crypto().VerifyQuorumCert(qc) {
		s.mod.Logger().Info("updateHighQC: QC could not be verified!")
		return
	}

	newBlock, ok := s.mod.BlockChain().Get(qc.BlockHash())
	if !ok {
		s.mod.Logger().Info("updateHighQC: Could not find block referenced by new QC!")
		return
	}

	oldBlock, ok := s.mod.BlockChain().Get(s.highQC.BlockHash())
	if !ok {
		s.mod.Logger().Panic("Block from the old highQC missing from chain")
	}

	if newBlock.View() > oldBlock.View() {
		s.highQC = qc
		s.leafBlock = newBlock
	}
}

var _ consensus.Synchronizer = (*Synchronizer)(nil)
