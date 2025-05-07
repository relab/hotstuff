package kauri

import (
	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/kauripb"
	"github.com/relab/hotstuff/service/server"
)

type serviceImpl struct {
	eventLoop *eventloop.EventLoop
}

// RegisterService registers a service implementation for Gorums which allows sending ContributionRecvEvent.
func RegisterService(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	server *server.Server,
) {
	i := &serviceImpl{eventLoop: eventLoop}
	logger.Infof("Kauripb: Service registered.")
	kauripb.RegisterKauriServer(server.GetGorumsServer(), i)
}

func (i serviceImpl) SendContribution(_ gorums.ServerCtx, request *kauripb.Contribution) {
	i.eventLoop.AddEvent(ContributionRecvEvent{Contribution: request})
}

// ContributionRecvEvent is raised when a contribution is received.
type ContributionRecvEvent struct {
	Contribution *kauripb.Contribution
}
