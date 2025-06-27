package network

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
)

// replicaNode provides methods used by hotstuff to send messages to replicas.
type replicaNode struct {
	eventLoop *eventloop.EventLoop
	node      *hotstuffpb.Node
	id        hotstuff.ID
	pubKey    hotstuff.PublicKey
	md        map[string]string
}

// vote sends the partial certificate to the other replica.
func (r *replicaNode) vote(cert hotstuff.PartialCert) {
	if r.node == nil {
		return
	}
	ctx, cancel := r.eventLoop.TimeoutContext()
	defer cancel()
	pCert := hotstuffpb.PartialCertToProto(cert)
	r.node.Vote(ctx, pCert)
}

// newView sends the quorum certificate to the other replica.
func (r *replicaNode) newView(msg hotstuff.SyncInfo) {
	if r.node == nil {
		return
	}
	ctx, cancel := r.eventLoop.TimeoutContext()
	defer cancel()
	r.node.NewView(ctx, hotstuffpb.SyncInfoToProto(msg))
}
