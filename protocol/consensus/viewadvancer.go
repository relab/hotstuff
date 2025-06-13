package consensus

import (
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/cert"
)

type ViewAdvancer struct {
	config         *core.RuntimeConfig
	logger         logging.Logger
	eventLoop      *eventloop.EventLoop
	state          *ViewStates
	auth           *cert.Authority
	duration       modules.ViewDuration
	leaderRotation modules.LeaderRotation
	sender         modules.Sender

	timer       oneShotTimer
	lastTimeout *hotstuff.TimeoutMsg
}

func NewViewAdvancer(
	config *core.RuntimeConfig,
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	state *ViewStates,
	auth *cert.Authority,
	duration modules.ViewDuration,
	leaderRotation modules.LeaderRotation,
	sender modules.Sender,
) *ViewAdvancer {
	return &ViewAdvancer{
		config:         config,
		logger:         logger,
		eventLoop:      eventLoop,
		state:          state,
		auth:           auth,
		duration:       duration,
		leaderRotation: leaderRotation,
		sender:         sender,
	}
}

// A oneShotTimer is a timer that should not be reused.
type oneShotTimer struct {
	timerDoNotUse *time.Timer
}

func (t oneShotTimer) Stop() bool {
	return t.timerDoNotUse.Stop()
}

func (s *ViewAdvancer) StartTimeoutTimer() {
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

func (s *ViewAdvancer) StopTimeoutTimer() {
	s.timer.Stop()
}

// advanceView attempts to advance to the next view using the given QC.
// qc must be either a regular quorum certificate, or a timeout certificate.
func (s *ViewAdvancer) Advance(syncInfo hotstuff.SyncInfo) (newSyncInfo hotstuff.SyncInfo, newView hotstuff.View) { // nolint: gocyclo
	view := hotstuff.View(0)
	timeout := false

	// check for a TC
	if tc, ok := syncInfo.TC(); ok {
		if err := s.auth.VerifyTimeoutCert(s.config.QuorumSize(), tc); err != nil {
			s.logger.Info("Timeout certificate could not be verified: %v", err)
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
		highQC, err := s.auth.VerifyAggregateQC(s.config.QuorumSize(), aggQC)
		if err != nil {
			s.logger.Info("advanceView: Agg-qc could not be verified: %v", err)
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
			s.logger.Info("advanceView: QC could not be verified: %v", err)
			return
		}
	}

	if haveQC {
		err := s.state.UpdateHighQC(qc)
		if err != nil {
			s.logger.Warnf("advanceView: Failed to update high-qc: %v", err)
		} else {
			s.logger.Debug("advanceView: High-qc updated")
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

	s.StopTimeoutTimer()

	if !timeout {
		s.duration.ViewSucceeded()
	}

	newView = s.state.View() + 1

	s.state.UpdateView(newView)

	s.lastTimeout = nil
	s.duration.ViewStarted()

	s.StartTimeoutTimer()

	s.logger.Debugf("advanceView: Advanced to view %d", newView)
	s.eventLoop.AddEvent(hotstuff.ViewChangeEvent{View: newView, Timeout: timeout})
	return
}
