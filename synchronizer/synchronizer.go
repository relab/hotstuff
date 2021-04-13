package synchronizer

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/relab/hotstuff"
)

// Synchronizer is a dumb implementation of the hotstuff.ViewSynchronizer interface.
// It does not do anything to ensure synchronization, it simply makes the local replica
// propose at the correct time, and send new view messages in case of a timeout.
type Synchronizer struct {
	mod *hotstuff.HotStuff

	currentView  hotstuff.View
	latestCommit hotstuff.View // the view in which the latest commit happened.
	highTC       hotstuff.TimeoutCert
	highQC       hotstuff.QuorumCert
	leafBlock    *hotstuff.Block

	viewCtx   context.Context
	cancelCtx context.CancelFunc
	timeout   hotstuff.ExponentialTimeout
	timer     *time.Timer
	timeouts  map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg
}

// InitModule initializes the synchronizer with the given HotStuff instance.
func (s *Synchronizer) InitModule(hs *hotstuff.HotStuff) {
	s.mod = hs
	var err error
	s.highQC, err = s.mod.Crypto().CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty quorum cert for genesis block: %v", err))
	}
	s.highTC, err = s.mod.Crypto().CreateTimeoutCert(hotstuff.View(0), []hotstuff.TimeoutMsg{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty timeout cert for view 0: %v", err))
	}
}

// New creates a new Synchronizer.
func New(timeout hotstuff.ExponentialTimeout) hotstuff.ViewSynchronizer {
	return &Synchronizer{
		leafBlock:    hotstuff.GetGenesis(),
		currentView:  1,
		latestCommit: 0,
		viewCtx:      context.Background(),
		cancelCtx:    func() {},
		timeout:      timeout,
		timeouts:     make(map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg),
		timer:        time.AfterFunc(0, func() {}), // dummy timer that will be replaced after start() is called
	}
}

// Start starts the view timeout timer, and makes a proposal if the local replica is the leader.
func (s *Synchronizer) Start() {
	s.timer = time.AfterFunc(s.viewDuration(s.currentView), s.onLocalTimeout)
}

// Stop stops the view timeout timer.
func (s *Synchronizer) Stop() {
	s.timer.Stop()
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

func (s *Synchronizer) viewDuration(view hotstuff.View) time.Duration {
	pow := view - s.latestCommit
	if pow > hotstuff.View(s.timeout.MaxExponent) {
		pow = hotstuff.View(s.timeout.MaxExponent)
	}
	multiplier := math.Pow(float64(s.timeout.ExponentBase), float64(pow))
	return time.Duration(
		math.Ceil(float64(s.timeout.Base) * multiplier),
	)
}

// SyncInfo returns the highest known QC or TC.
func (s *Synchronizer) SyncInfo() hotstuff.SyncInfo {
	qc := s.highQC
	qcBlock, ok := s.mod.BlockChain().Get(qc.BlockHash())
	if !ok {
		// TODO
		panic("highQC block missing")
	}
	if qcBlock.View() >= s.highTC.View() {
		return hotstuff.SyncInfoWithQC(s.highQC)
	}
	return hotstuff.SyncInfoWithTC(s.highTC)
}

func (s *Synchronizer) onLocalTimeout() {
	s.mod.Logger().Debugf("OnLocalTimeout: %v", s.currentView)
	sig, err := s.mod.Crypto().Sign(s.currentView.ToHash())
	if err != nil {
		s.mod.Logger().Warnf("Failed to sign view: %v", err)
		return
	}
	timeoutMsg := hotstuff.TimeoutMsg{
		ID:        s.mod.ID(),
		View:      s.currentView,
		SyncInfo:  s.SyncInfo(),
		Signature: sig,
	}
	s.mod.Manager().Timeout(timeoutMsg)
	s.mod.EventLoop().AddEvent(timeoutMsg)
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

	verifier := s.mod.Crypto()
	if !verifier.Verify(timeout.Signature, timeout.View.ToHash()) {
		return
	}
	s.mod.Logger().Debug("OnRemoteTimeout: ", timeout)

	// This has to be done in this function instead of onLocalTimeout in order to avoid
	// race conditions.
	if timeout.ID == s.mod.ID() {
		// stop voting for current view
		s.mod.Consensus().IncreaseLastVotedView(s.currentView)
	}

	timeouts, ok := s.timeouts[timeout.View]
	if !ok {
		timeouts = make(map[hotstuff.ID]hotstuff.TimeoutMsg)
		s.timeouts[timeout.View] = timeouts
	}

	if _, ok := timeouts[timeout.ID]; !ok {
		timeouts[timeout.ID] = timeout
	}

	if len(timeouts) < s.mod.Manager().QuorumSize() {
		return
	}

	// TODO: should probably change CreateTimeoutCert and maybe also CreateQuorumCert
	// to use maps instead of slices
	timeoutList := make([]hotstuff.TimeoutMsg, 0, len(timeouts))
	for _, t := range timeouts {
		timeoutList = append(timeoutList, t)
	}

	signer := s.mod.Crypto()
	tc, err := signer.CreateTimeoutCert(s.currentView, timeoutList)
	if err != nil {
		s.mod.Logger().Debugf("Failed to create timeout certificate: %v", err)
		return
	}
	delete(s.timeouts, timeout.View)

	s.AdvanceView(hotstuff.SyncInfoWithTC(tc))
}

// OnNewView handles an incoming NewViewMsg
func (s *Synchronizer) OnNewView(newView hotstuff.NewViewMsg) {
	s.AdvanceView(newView.SyncInfo)
}

// AdvanceView attempts to advance to the next view using the given QC.
// qc must be either a regular quorum certificate, or a timeout certificate.
func (s *Synchronizer) AdvanceView(syncInfo hotstuff.SyncInfo) {
	var v hotstuff.View
	if tc, ok := syncInfo.TC(); ok {
		v = tc.View()
		if v > s.highTC.View() {
			s.highTC = tc
		}
	} else if qc, ok := syncInfo.QC(); ok {
		s.UpdateHighQC(qc)
		b, ok := s.mod.BlockChain().Get(qc.BlockHash())
		if !ok {
			return
		}
		v = b.View()
		if s.latestCommit < v {
			s.latestCommit = v
		}
	}

	if v < s.currentView {
		return
	}

	s.currentView = v + 1
	s.timer.Reset(s.viewDuration(s.currentView))

	// cancel the old view context and set up the next one
	s.cancelCtx()
	s.viewCtx, s.cancelCtx = context.WithCancel(context.Background())

	leader := s.mod.LeaderRotation().GetLeader(s.currentView)
	if leader == s.mod.ID() {
		s.mod.Consensus().Propose()
	} else if replica, ok := s.mod.Manager().Replica(leader); ok {
		replica.NewView(syncInfo)
	}
}

// UpdateHighQC updates HighQC if the given qc is higher than the old HighQC.
func (s *Synchronizer) UpdateHighQC(qc hotstuff.QuorumCert) {
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

var _ hotstuff.ViewSynchronizer = (*Synchronizer)(nil)
