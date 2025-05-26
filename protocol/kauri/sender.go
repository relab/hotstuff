package kauri

import (
	"context"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/proto/kauripb"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network"
)

type gorumsSender struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
	config    *core.RuntimeConfig
	base      *network.GorumsSender
	modules.Sender

	nodes   map[hotstuff.ID]*kauripb.Node
	senders []hotstuff.ID
	tree    *tree.Tree
}

func NewExtendedGorumsSender(
	eventLoop *eventloop.EventLoop,
	base *network.GorumsSender,
) modules.KauriSender {
	s := &gorumsSender{
		eventLoop: eventLoop,
		Sender:    base,
	}

	s.eventLoop.RegisterHandler(hotstuff.ReplicaConnectedEvent{}, func(_ any) {
		s.postInit()
	}, eventloop.Prioritize())
	return s
}

func (s *gorumsSender) postInit() {
	kauriCfg := kauripb.ConfigurationFromRaw(s.base.GorumsConfig(), nil)
	for _, n := range kauriCfg.Nodes() {
		s.nodes[hotstuff.ID(n.ID())] = n
	}
	s.tree = s.config.Tree()
	s.senders = make([]hotstuff.ID, 0)
}

func (k *gorumsSender) SendContributionToParent(view hotstuff.View, qc hotstuff.QuorumSignature) {
	parent, ok := k.tree.Parent()
	if ok {
		node, isPresent := k.nodes[parent]
		if isPresent {
			node.SendContribution(context.Background(), &kauripb.Contribution{
				ID:        uint32(k.config.ID()),
				Signature: hotstuffpb.QuorumSignatureToProto(qc),
				View:      uint64(view),
			})
		}
	}
}
