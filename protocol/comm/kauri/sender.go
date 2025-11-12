package kauri

import (
	"context"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/proto/kauripb"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/network"
)

type KauriGorumsSender struct {
	config *core.RuntimeConfig
	core.Sender

	nodes map[hotstuff.ID]*kauripb.Node
	tree  *tree.Tree
}

func WrapGorumsSender(
	el *eventloop.EventLoop,
	config *core.RuntimeConfig,
	base *network.GorumsSender,
) *KauriGorumsSender {
	s := &KauriGorumsSender{
		config: config,
		Sender: base, // important: extend the base

		nodes: make(map[hotstuff.ID]*kauripb.Node),
		tree:  config.Tree(),
	}
	eventloop.Register(el, func(_ hotstuff.ReplicaConnectedEvent) {
		kauriCfg := kauripb.ConfigurationFromRaw(base.GorumsConfig(), nil)
		for _, n := range kauriCfg.Nodes() {
			s.nodes[hotstuff.ID(n.ID())] = n
		}
	}, eventloop.Prioritize())
	return s
}

func (k *KauriGorumsSender) SendContributionToParent(view hotstuff.View, qc hotstuff.QuorumSignature) {
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

var _ core.KauriSender = (*KauriGorumsSender)(nil)
