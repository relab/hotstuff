// Package kauri contains the implementation of the Kauri protocol
package kauri

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/proto/kauripb"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
)

const ModuleName = "kauri"

// Kauri structure contains the modules for kauri protocol implementation.
type Kauri struct {
	logger     logging.Logger
	eventLoop  *eventloop.EventLoop
	config     *core.RuntimeConfig
	blockChain *blockchain.BlockChain
	auth       *certauth.CertAuthority
	sender     *network.GorumsSender

	aggContrib  hotstuff.QuorumSignature
	aggSent     bool
	blockHash   hotstuff.Hash
	currentView hotstuff.View
	initDone    bool
	nodes       map[hotstuff.ID]*kauripb.Node
	senders     []hotstuff.ID
	tree        *tree.Tree
}

// New initializes the kauri structure
func New(
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	blockChain *blockchain.BlockChain,
	auth *certauth.CertAuthority,
	sender *network.GorumsSender,
) *Kauri {
	k := &Kauri{
		blockChain: blockChain,
		auth:       auth,
		config:     config,
		eventLoop:  eventLoop,
		sender:     sender,
		logger:     logger,

		nodes: make(map[hotstuff.ID]*kauripb.Node),
		tree:  nil, // is set later
	}
	k.eventLoop.RegisterHandler(hotstuff.ReplicaConnectedEvent{}, func(_ any) {
		k.postInit()
	}, eventloop.Prioritize())
	k.eventLoop.RegisterHandler(ContributionRecvEvent{}, func(event any) {
		k.OnContributionRecv(event.(ContributionRecvEvent))
	})
	k.eventLoop.RegisterHandler(WaitTimerExpiredEvent{}, func(event any) {
		k.OnWaitTimerExpired(event.(WaitTimerExpiredEvent))
	})
	return k
}

func (k *Kauri) postInit() {
	k.logger.Debug("Kauri: Initializing")
	k.initializeConfiguration()
}

// TODO(AlanRostem): avoid using raw nodes.
func (k *Kauri) initializeConfiguration() {
	kauriCfg := kauripb.ConfigurationFromRaw(k.sender.GorumsConfig(), nil)
	for _, n := range kauriCfg.Nodes() {
		k.nodes[hotstuff.ID(n.ID())] = n
	}
	k.tree = k.config.Tree()
	k.initDone = true
	k.senders = make([]hotstuff.ID, 0)
}

// Begin starts dissemination of proposal and aggregation of votes.
func (k *Kauri) Begin(pc hotstuff.PartialCert, p hotstuff.ProposeMsg) {
	if !k.initDone {
		k.eventLoop.DelayUntil(network.ConnectedEvent{}, func() { k.Begin(pc, p) })
		return
	}
	k.reset()
	k.blockHash = pc.BlockHash()
	k.currentView = p.Block.View()
	k.aggContrib = pc.Signature()
	k.SendProposalToChildren(p)
}

func (k *Kauri) reset() {
	k.aggContrib = nil
	k.senders = make([]hotstuff.ID, 0)
	k.aggSent = false
}

func (k *Kauri) ExtOnPropose(proposal hotstuff.ProposeMsg, pc hotstuff.PartialCert) {
	k.Begin(pc, proposal)
}

func (k *Kauri) ExtDisseminatePropose(proposal hotstuff.ProposeMsg, pc hotstuff.PartialCert) {
	k.Begin(pc, proposal)
}

func (k *Kauri) WaitToAggregate() {
	view := k.currentView
	time.Sleep(k.tree.WaitTime())
	k.eventLoop.AddEvent(WaitTimerExpiredEvent{currentView: view})
}

// SendProposalToChildren sends the proposal to the children.
func (k *Kauri) SendProposalToChildren(p hotstuff.ProposeMsg) {
	children := k.tree.ReplicaChildren()
	if len(children) != 0 {
		config, err := k.sender.Sub(children)
		if err != nil {
			k.logger.Errorf("Unable to send the proposal to children: %v", err)
			return
		}
		k.logger.Debug("Sending proposal to children ", children)
		config.Propose(p)
		go k.WaitToAggregate()
	} else {
		k.SendContributionToParent()
		k.aggSent = true
	}
}

// OnContributionRecv is invoked upon receiving the vote for aggregation.
func (k *Kauri) OnContributionRecv(event ContributionRecvEvent) {
	if k.currentView != hotstuff.View(event.Contribution.View) {
		return
	}
	contribution := event.Contribution
	k.logger.Debugf("Processing the contribution from %d", contribution.ID)
	currentSignature := hotstuffpb.QuorumSignatureFromProto(contribution.Signature)
	err := k.mergeContribution(currentSignature)
	if err != nil {
		k.logger.Errorf("Failed to merge contribution from %d: %v", contribution.ID, err)
		return
	}
	k.senders = append(k.senders, hotstuff.ID(contribution.ID))
	if isSubSet(k.tree.SubTree(), k.senders) {
		k.SendContributionToParent()
		k.aggSent = true
	}
}

// SendContributionToParent sends contribution to the parent node.
func (k *Kauri) SendContributionToParent() {
	parent, ok := k.tree.Parent()
	if ok {
		node, isPresent := k.nodes[parent]
		if isPresent {
			node.SendContribution(context.Background(), &kauripb.Contribution{
				ID:        uint32(k.config.ID()),
				Signature: hotstuffpb.QuorumSignatureToProto(k.aggContrib),
				View:      uint64(k.currentView),
			})
		}
	}
}

func (k *Kauri) OnWaitTimerExpired(event WaitTimerExpiredEvent) {
	if k.currentView != event.currentView {
		return
	}
	if !k.aggSent {
		k.SendContributionToParent()
		k.reset()
	}
}

// canMergeContributions returns nil if the contributions are non-overlapping.
func canMergeContributions(a, b hotstuff.QuorumSignature) error {
	if a == nil || b == nil {
		return errors.New("cannot merge nil contributions")
	}
	canMerge := true
	a.Participants().RangeWhile(func(i hotstuff.ID) bool {
		// cannot merge a and b if both contain a contribution from the same ID.
		canMerge = !b.Participants().Contains(i)
		return canMerge // exit the range-while loop if canMerge is false
	})
	if !canMerge {
		return errors.New("cannot merge overlapping contributions")
	}
	return nil
}

func (k *Kauri) mergeContribution(currentSignature hotstuff.QuorumSignature) error {
	block, ok := k.blockChain.Get(k.blockHash)
	if !ok {
		return fmt.Errorf("failed to fetch block %v", k.blockHash)
	}
	if !k.auth.Verify(currentSignature, block.ToBytes()) {
		return fmt.Errorf("cannot verify contribution for view %d from participants %v", k.currentView, currentSignature.Participants())
	}
	if k.aggContrib == nil {
		// first contribution
		k.aggContrib = currentSignature
		return nil
	}
	if err := canMergeContributions(currentSignature, k.aggContrib); err != nil {
		return err
	}
	combSignature, err := k.auth.Combine(currentSignature, k.aggContrib)
	if err != nil {
		return fmt.Errorf("failed to combine signatures: %v", err)
	}
	k.aggContrib = combSignature
	if combSignature.Participants().Len() >= k.config.QuorumSize() {
		k.logger.Debug("Aggregated Complete QC and sending the event")
		k.eventLoop.AddEvent(hotstuff.NewViewMsg{
			SyncInfo: hotstuff.NewSyncInfo().WithQC(hotstuff.NewQuorumCert(
				k.aggContrib,
				k.currentView,
				k.blockHash,
			)),
		})
	} // else, wait for more contributions
	return nil
}

// isSubSet returns true if a is a subset of b.
func isSubSet(a, b []hotstuff.ID) bool {
	c := hotstuff.NewIDSet()
	for _, id := range b {
		c.Add(id)
	}
	for _, id := range a {
		if !c.Contains(id) {
			return false
		}
	}
	return true
}

type WaitTimerExpiredEvent struct {
	currentView hotstuff.View
}

var _ modules.ExtProposeHandler = (*Kauri)(nil)
