// Package synchronizer implements the synchronizer module.
package synchronizer

import (
	"context"
	"fmt"
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
	currentViews   map[hotstuff.ChainNumber]hotstuff.View
	highTC         map[hotstuff.ChainNumber]hotstuff.TimeoutCert
	highQC         map[hotstuff.ChainNumber]hotstuff.QuorumCert
	lastTimeouts   map[hotstuff.ChainNumber]*hotstuff.TimeoutMsg
	// A pointer to the last timeout message that we sent.
	// If a timeout happens again before we advance to the next view,
	// we will simply send this timeout again.
	//lastTimeout *hotstuff.TimeoutMsg

	duration map[hotstuff.ChainNumber]ViewDuration
	timer    map[hotstuff.ChainNumber]*time.Timer

	viewCtx   map[hotstuff.ChainNumber]context.Context // a context that is cancelled at the end of the current view
	cancelCtx map[hotstuff.ChainNumber]context.CancelFunc

	// map of collected timeout messages per view
	timeouts map[hotstuff.ChainNumber]map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg
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
	)

	s.eventLoop.RegisterHandler(TimeoutEvent{}, func(event any) {
		timeoutEvent := event.(TimeoutEvent)
		if s.currentViews[timeoutEvent.ChainNumber] == timeoutEvent.View {
			s.OnLocalTimeout(timeoutEvent.ChainNumber)
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

	s.eventLoop.RegisterHandler(DelayedProposeEvent{}, func(event any) {
		proposeEvent := event.(DelayedProposeEvent)
		s.onDelayProposeEvent(proposeEvent)
	})
	for i := 1; i <= hotstuff.NumberOfChains; i++ {

		highQC, err := s.crypto.CreateQuorumCert(hotstuff.GetGenesis(hotstuff.ChainNumber(i)), []hotstuff.PartialCert{})
		if err != nil {
			panic(fmt.Errorf("unable to create empty quorum cert for genesis block: %v", err))
		}
		highTC, err := s.crypto.CreateTimeoutCert(hotstuff.View(0), []hotstuff.TimeoutMsg{})
		if err != nil {
			panic(fmt.Errorf("unable to create empty timeout cert for view 0: %v", err))
		}
		s.highQC[hotstuff.ChainNumber(i)] = highQC
		s.highTC[hotstuff.ChainNumber(i)] = highTC
	}
}

// New creates a new Synchronizer.
func New(viewDuration ViewDuration) modules.Synchronizer {
	currentViews := make(map[hotstuff.ChainNumber]hotstuff.View)
	viewCtx := make(map[hotstuff.ChainNumber]context.Context)
	viewDurations := make(map[hotstuff.ChainNumber]ViewDuration)
	cancelCtx := make(map[hotstuff.ChainNumber]context.CancelFunc)
	timers := make(map[hotstuff.ChainNumber]*time.Timer)
	highTC := make(map[hotstuff.ChainNumber]hotstuff.TimeoutCert)
	highQC := make(map[hotstuff.ChainNumber]hotstuff.QuorumCert)
	for i := 1; i <= hotstuff.NumberOfChains; i++ {
		currentViews[hotstuff.ChainNumber(i)] = 1
		timers[hotstuff.ChainNumber(i)] = time.AfterFunc(0, func() {})
		ctx, cancel := context.WithCancel(context.Background())
		viewCtx[hotstuff.ChainNumber(i)] = ctx
		cancelCtx[hotstuff.ChainNumber(i)] = cancel
		tmpViewDuration := viewDuration.CreateNewDuration()
		viewDurations[hotstuff.ChainNumber(i)] = tmpViewDuration
	}

	return &Synchronizer{
		currentViews: currentViews,
		viewCtx:      viewCtx,
		cancelCtx:    cancelCtx,
		duration:     viewDurations,
		timer:        timers, // dummy timer that will be replaced after start() is called
		timeouts:     make(map[hotstuff.ChainNumber]map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg),
		lastTimeouts: make(map[hotstuff.ChainNumber]*hotstuff.TimeoutMsg),
		highTC:       highTC,
		highQC:       highQC,
	}
}

func (s *Synchronizer) addTimeoutEvent(chainNumber hotstuff.ChainNumber) func() {
	// The event loop will execute onLocalTimeout for us.
	return func() {
		if chainNumber <= hotstuff.NumberOfChains {
			s.cancelCtx[chainNumber]()
			s.eventLoop.AddEvent(TimeoutEvent{View: s.currentViews[chainNumber], ChainNumber: chainNumber})
		} else {
			s.logger.Info("out of order i ", chainNumber)
		}
	}
}

// Start starts the synchronizer with the given context.
func (s *Synchronizer) Start(ctx context.Context) {
	for i := 1; i <= hotstuff.NumberOfChains; i++ {
		s.logger.Info(" cancel ctx is ", s.cancelCtx, i, s.duration[hotstuff.ChainNumber(i)])
		s.timer[hotstuff.ChainNumber(i)] = time.AfterFunc(s.duration[hotstuff.ChainNumber(i)].Duration(), s.addTimeoutEvent(hotstuff.ChainNumber(i)))
		// start the initial proposal
		if s.currentViews[hotstuff.ChainNumber(i)] == 1 &&
			s.leaderRotation.GetLeader(s.currentViews[hotstuff.ChainNumber(i)]) == s.opts.ID() {
			s.consensus.Propose(hotstuff.ChainNumber(i), s.SyncInfo(hotstuff.ChainNumber(i)))
		}
	}
	go func() {
		<-ctx.Done()
		for i := 1; i <= hotstuff.NumberOfChains; i++ {
			s.timer[hotstuff.ChainNumber(i)].Stop()
		}
	}()
}

// HighQC returns the highest known QC.
func (s *Synchronizer) HighQC(chainNumber hotstuff.ChainNumber) hotstuff.QuorumCert {
	return s.highQC[chainNumber]
}

// View returns the current view.
func (s *Synchronizer) View(chainNumber hotstuff.ChainNumber) hotstuff.View {
	return s.currentViews[chainNumber]
}

// ViewContext returns a context that is cancelled at the end of the view.
func (s *Synchronizer) ViewContext(chainNumber hotstuff.ChainNumber) context.Context {
	return s.viewCtx[chainNumber]
}

// SyncInfo returns the highest known QC or TC.
func (s *Synchronizer) SyncInfo(chainNumber hotstuff.ChainNumber) hotstuff.SyncInfo {
	return hotstuff.NewSyncInfo(chainNumber).WithQC(s.highQC[chainNumber]).WithTC(s.highTC[chainNumber])
}

// OnLocalTimeout is called when a local timeout happens.
func (s *Synchronizer) OnLocalTimeout(chainNumber hotstuff.ChainNumber) {
	// Reset the timer and ctx here so that we can get a new timeout in the same view.
	// I think this is necessary to ensure that we can keep sending the same timeout message
	// until we get a timeout certificate.
	//
	// TODO: figure out the best way to handle this context and timeout.
	if s.viewCtx[chainNumber].Err() != nil {
		s.newCtx(chainNumber, s.duration[chainNumber].Duration())
	}
	s.timer[chainNumber].Reset(s.duration[chainNumber].Duration())

	if s.lastTimeouts[chainNumber] != nil && s.lastTimeouts[chainNumber].View == s.currentViews[chainNumber] {
		s.configuration.Timeout(*s.lastTimeouts[chainNumber])
		return
	}

	s.duration[chainNumber].ViewTimeout() // increase the duration of the next view
	view := s.currentViews[chainNumber]
	s.logger.Debugf("OnLocalTimeout: view  %v chain number %v ", view, chainNumber)

	sig, err := s.crypto.Sign(view.ToBytes())
	if err != nil {
		s.logger.Warnf("Failed to sign view: %v", err)
		return
	}
	timeoutMsg := hotstuff.TimeoutMsg{
		ID:            s.opts.ID(),
		View:          view,
		SyncInfo:      s.SyncInfo(chainNumber),
		ViewSignature: sig,
		ChainNumber:   chainNumber,
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
	s.lastTimeouts[chainNumber] = &timeoutMsg
	// stop voting for current view
	s.consensus.StopVoting(s.currentViews[chainNumber], timeoutMsg.ChainNumber)

	s.configuration.Timeout(timeoutMsg)
	s.OnRemoteTimeout(timeoutMsg)
}

// OnRemoteTimeout handles an incoming timeout from a remote replica.
func (s *Synchronizer) OnRemoteTimeout(timeout hotstuff.TimeoutMsg) {

	defer func() {
		// cleanup old timeouts
		timeoutMsg := s.timeouts[timeout.ChainNumber]
		for view := range timeoutMsg {
			if view < s.currentViews[timeout.ChainNumber] {
				delete(s.timeouts[timeout.ChainNumber], view)
			}
		}
	}()

	verifier := s.crypto
	if !verifier.Verify(timeout.ViewSignature, timeout.View.ToBytes()) {
		return
	}
	s.logger.Debug("OnRemoteTimeout: ", timeout)

	s.AdvanceView(timeout.SyncInfo)
	_, ok := s.timeouts[timeout.ChainNumber]
	if !ok {
		s.timeouts[timeout.ChainNumber] = make(map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg)
	}
	timeouts, ok := s.timeouts[timeout.ChainNumber][timeout.View]
	if !ok {
		timeouts = make(map[hotstuff.ID]hotstuff.TimeoutMsg)
		s.timeouts[timeout.ChainNumber][timeout.View] = timeouts
	}

	if _, ok := timeouts[timeout.ID]; !ok {
		timeouts[timeout.ID] = timeout
	}

	if len(timeouts) < s.configuration.QuorumSize() {
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

	si := s.SyncInfo(timeout.ChainNumber).WithTC(tc)

	if s.opts.ShouldUseAggQC() {
		aggQC, err := s.crypto.CreateAggregateQC(s.currentViews[timeout.ChainNumber],
			timeoutList)
		if err != nil {
			s.logger.Debugf("Failed to create aggregateQC: %v", err)
		} else {
			si = si.WithAggQC(aggQC)
		}
	}

	delete(s.timeouts[timeout.ChainNumber], timeout.View)

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
	chainNumber := syncInfo.GetChainNumber()
	// check for a TC
	if tc, ok := syncInfo.TC(); ok {
		if !s.crypto.VerifyTimeoutCert(tc) {
			s.logger.Info("Timeout Certificate could not be verified!")
			return
		}
		s.updateHighTC(chainNumber, tc)
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
		s.updateHighQC(chainNumber, qc)
		// if there is both a TC and a QC, we use the QC if its view is greater or equal to the TC.
		if qc.View() >= v {
			v = qc.View()
			timeout = false
		}
	}

	if v < s.currentViews[chainNumber] {
		return
	}

	s.timer[chainNumber].Stop()
	s.logger.Debugf("Timer stopped for chain %d view %d", chainNumber, v)
	if !timeout {
		s.duration[chainNumber].ViewSucceeded()
	}

	s.currentViews[chainNumber] = v + 1
	s.lastTimeouts[chainNumber] = nil
	s.duration[chainNumber].ViewStarted()

	duration := s.duration[chainNumber].Duration()
	// cancel the old view context and set up the next one
	s.newCtx(chainNumber, duration)
	s.timer[chainNumber].Reset(duration)

	s.logger.Debugf("advanced to view %d for chain %d", s.currentViews[chainNumber], chainNumber)
	s.eventLoop.AddEvent(ViewChangeEvent{View: s.currentViews[chainNumber], Timeout: timeout})

	leader := s.leaderRotation.GetLeader(s.currentViews[chainNumber])
	if leader == s.opts.ID() {
		// if !s.checkCurrentViews(chainNumber) {
		// 	s.logger.Debug("Sync views of chains failed delaying the proposal")
		// 	s.eventLoop.DelayUntil(ViewChangeEvent{}, DelayedProposeEvent{SyncInfo: syncInfo,
		// 		ChainNumber: chainNumber})
		// 	return
		// }
		s.consensus.Propose(chainNumber, syncInfo)
	} else if replica, ok := s.configuration.Replica(leader); ok {
		replica.NewView(syncInfo)
	}
}

func (s *Synchronizer) checkCurrentViews(chainNumber hotstuff.ChainNumber) bool {
	for i := 1; i <= hotstuff.NumberOfChains; i++ {
		if i == int(chainNumber) {
			continue
		}
		if s.currentViews[chainNumber] > s.currentViews[hotstuff.ChainNumber(i)]+4 {
			s.logger.Debugf("Chain number %d has view %d and chain number %d has view %d",
				chainNumber, s.currentViews[chainNumber], i, s.currentViews[hotstuff.ChainNumber(i)])
			return false
		}
	}
	return true
}

// updateHighQC attempts to update the highQC, but does not verify the qc first.
// This method is meant to be used instead of the exported UpdateHighQC internally
// in this package when the qc has already been verified.
func (s *Synchronizer) updateHighQC(chainNumber hotstuff.ChainNumber, qc hotstuff.QuorumCert) {
	newBlock, ok := s.blockChain.Get(chainNumber, qc.BlockHash())
	if !ok {
		s.logger.Info("updateHighQC: Could not find block referenced by new QC!")
		return
	}

	if newBlock.View() > s.highQC[chainNumber].View() {
		s.highQC[chainNumber] = qc
		s.logger.Debug("HighQC updated")
	}
}

// updateHighTC attempts to update the highTC, but does not verify the tc first.
func (s *Synchronizer) updateHighTC(chainNumber hotstuff.ChainNumber, tc hotstuff.TimeoutCert) {
	if tc.View() > s.highTC[chainNumber].View() {
		s.highTC[chainNumber] = tc
		s.logger.Debug("HighTC updated")
	}
}

func (s *Synchronizer) onDelayProposeEvent(event DelayedProposeEvent) {
	if s.checkCurrentViews(event.ChainNumber) {
		s.consensus.Propose(event.ChainNumber, event.SyncInfo)
	} else {
		s.logger.Debug("Sync views of chains failed delaying the proposal again")
		s.eventLoop.DelayUntil(ViewChangeEvent{}, event)
	}
}

func (s *Synchronizer) newCtx(chainNumber hotstuff.ChainNumber, duration time.Duration) {
	s.cancelCtx[chainNumber]()
	viewCtx, cancelCtx := context.WithTimeout(context.Background(), duration)
	s.cancelCtx[chainNumber] = cancelCtx
	s.viewCtx[chainNumber] = viewCtx

}

var _ modules.Synchronizer = (*Synchronizer)(nil)

// ViewChangeEvent is sent on the eventloop whenever a view change occurs.
type ViewChangeEvent struct {
	View    hotstuff.View
	Timeout bool
}

// TimeoutEvent is sent on the eventloop when a local timeout occurs.
type TimeoutEvent struct {
	View        hotstuff.View
	ChainNumber hotstuff.ChainNumber
}

type DelayedProposeEvent struct {
	SyncInfo    hotstuff.SyncInfo
	ChainNumber hotstuff.ChainNumber
}
