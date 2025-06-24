package kauri

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/internal/proto/kauripb"
)

type kauriServiceImpl struct {
	eventLoop *eventloop.EventLoop
}

// RegisterService registers a service implementation for Gorums which allows sending ContributionRecvEvent.
func RegisterService(
	eventLoop *eventloop.EventLoop,
	gorumsSrv *gorums.Server,
) {
	i := &kauriServiceImpl{eventLoop: eventLoop}
	kauripb.RegisterKauriServer(gorumsSrv, i)
}

func (i kauriServiceImpl) SendContribution(_ gorums.ServerCtx, request *kauripb.Contribution) {
	i.eventLoop.AddEvent(ContributionRecvEvent{Contribution: request})
}

// ContributionRecvEvent is raised when a contribution is received.
type ContributionRecvEvent struct {
	Contribution *kauripb.Contribution
}
