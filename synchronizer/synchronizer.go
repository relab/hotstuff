package synchronizer

import (
	"fmt"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/logging"
)

var logger = logging.GetLogger()

// Synchronizer is a dumb implementation of the hotstuff.ViewSynchronizer interface.
// It does not do anything to ensure synchronization, it simply makes the local replica
// propose at the correct time, and send new view messages in case of a timeout.
type Synchronizer struct {
	mod *hotstuff.HotStuff

	highTC       hotstuff.TimeoutCert
	baseTimeout  time.Duration
	timer        *time.Timer
	currentView  hotstuff.View
	latestCommit hotstuff.View // the view in which the latest commit happened.
	timeouts     map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg
}

func (s *Synchronizer) InitModule(hs *hotstuff.HotStuff) {
	s.mod = hs
	var err error
	s.highTC, err = s.mod.Signer().CreateTimeoutCert(hotstuff.View(0), []hotstuff.TimeoutMsg{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty timeout cert for view 0: %v", err))
	}
}

// New creates a new Synchronizer.
func New(baseTimeout time.Duration) hotstuff.ViewSynchronizer {
	return &Synchronizer{
		currentView:  1,
		latestCommit: 0,
		baseTimeout:  baseTimeout,
		timeouts:     make(map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg),
		timer:        time.AfterFunc(0, func() {}), // dummy timer that will be replaced after start() is called
	}
}

// Start starts the view timeout timer, and makes a proposal if the local replica is the leader.
func (s *Synchronizer) Start() {
	s.timer = time.AfterFunc(s.viewDuration(s.currentView), s.onLocalTimeout)
	if s.mod.LeaderRotation().GetLeader(s.currentView) == s.mod.ID() {
		s.mod.Consensus().Propose()
	} else {
	}
}

// Stop stops the view timeout timer.
func (s *Synchronizer) Stop() {
	s.timer.Stop()
}

// View returns the current view.
func (s *Synchronizer) View() hotstuff.View {
	return s.currentView
}

func (s *Synchronizer) viewDuration(view hotstuff.View) time.Duration {
	// TODO: exponential backoff
	return s.baseTimeout
}

func (s *Synchronizer) SyncInfo() hotstuff.SyncInfo {
	qc := s.mod.Consensus().HighQC()
	qcBlock, ok := s.mod.BlockChain().Get(qc.BlockHash())
	if !ok {
		// TODO
		panic("highQC block missing")
	}
	if qcBlock.View() >= s.highTC.View() {
		return hotstuff.SyncInfo{QC: qc}
	}
	return hotstuff.SyncInfo{TC: s.highTC}
}

func (s *Synchronizer) onLocalTimeout() {
	// only run this once per view
	if m, ok := s.timeouts[s.currentView]; ok {
		if _, ok := m[s.mod.ID()]; ok {
			return
		}
	}
	// stop voting for current view
	s.mod.Consensus().IncreaseLastVotedView(s.currentView)
	sig, err := s.mod.Signer().Sign(s.currentView.ToHash())
	if err != nil {
		logger.Warnf("Failed to sign view: %v", err)
		return
	}
	timeoutMsg := hotstuff.TimeoutMsg{
		ID:        s.mod.ID(),
		View:      s.currentView,
		SyncInfo:  s.SyncInfo(),
		Signature: sig,
	}
	s.mod.Config().Timeout(timeoutMsg)
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

	verifier := s.mod.Verifier()
	if !verifier.Verify(timeout.Signature, timeout.View.ToHash()) {
		return
	}
	logger.Debug("OnRemoteTimeout: ", timeout)

	timeouts, ok := s.timeouts[timeout.View]
	if !ok {
		timeouts = make(map[hotstuff.ID]hotstuff.TimeoutMsg)
		s.timeouts[timeout.View] = timeouts
	}

	timeouts[timeout.ID] = timeout

	if len(timeouts) < s.mod.Config().QuorumSize() {
		return
	}

	// TODO: should probably change CreateTimeoutCert and maybe also CreateQuorumCert
	// to use maps instead of slices
	timeoutList := make([]hotstuff.TimeoutMsg, 0, len(timeouts))
	for _, t := range timeouts {
		timeoutList = append(timeoutList, t)
	}

	signer := s.mod.Signer()
	tc, err := signer.CreateTimeoutCert(s.currentView, timeoutList)
	if err != nil {
		logger.Debugf("Failed to create timeout certificate: %v", err)
		return
	}

	s.AdvanceView(hotstuff.SyncInfo{TC: tc})
}

// OnNewView handles an incoming NewViewMsg
func (s *Synchronizer) OnNewView(newView hotstuff.NewViewMsg) {
	s.AdvanceView(newView.SyncInfo)
}

// AdvanceView attempts to advance to the next view using the given QC.
// qc must be either a regular quorum certificate, or a timeout certificate.
func (s *Synchronizer) AdvanceView(syncInfo hotstuff.SyncInfo) {
	var v hotstuff.View
	if syncInfo.TC != nil {
		v = syncInfo.TC.View()
		if v > s.highTC.View() {
			s.highTC = syncInfo.TC
		}
	} else {
		b, ok := s.mod.BlockChain().Get(syncInfo.QC.BlockHash())
		if !ok {
			//TODO
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

	// TODO: stop timer
	s.currentView = v + 1
	s.timer.Reset(s.viewDuration(s.currentView))

	leader := s.mod.LeaderRotation().GetLeader(s.currentView)
	if leader == s.mod.ID() {
		s.mod.Consensus().Propose()
	} else if replica, ok := s.mod.Config().Replica(leader); ok {
		replica.NewView(syncInfo)
	}
}
