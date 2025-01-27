// Package kauri contains the implementation of the Kauri protocol
package kauri

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/proto/kauripb"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/netconfig"
	"github.com/relab/hotstuff/sender"
	"github.com/relab/hotstuff/server"
)

const ModuleName = "kauri"

// Kauri structure contains the modules for kauri protocol implementation.
type Kauri struct {
	crypto         modules.CryptoBase
	leaderRotation modules.LeaderRotation

	blockChain    *blockchain.BlockChain
	treeConfig    *core.TreeConfig
	opts          *core.Options
	eventLoop     *core.EventLoop
	configuration *netconfig.Config
	sender        *sender.Sender
	server        *server.Server
	logger        logging.Logger

	nodes       map[hotstuff.ID]*kauripb.Node
	tree        tree.Tree
	initDone    bool
	aggContrib  hotstuff.QuorumSignature
	blockHash   hotstuff.Hash
	currentView hotstuff.View
	senders     []hotstuff.ID
	aggSent     bool
}

// New initializes the kauri structure
func New(
	crypto modules.CryptoBase,
	leaderRotation modules.LeaderRotation,

	blockChain *blockchain.BlockChain,
	treeConfig *core.TreeConfig,
	opts *core.Options,
	eventLoop *core.EventLoop,
	configuration *netconfig.Config,
	sender *sender.Sender,
	server *server.Server,
	logger logging.Logger,
) *Kauri {
	k := &Kauri{
		crypto:         crypto,
		leaderRotation: leaderRotation,

		blockChain:    blockChain,
		treeConfig:    treeConfig,
		opts:          opts,
		eventLoop:     eventLoop,
		configuration: configuration,
		sender:        sender,
		server:        server,
		logger:        logger,

		nodes: make(map[hotstuff.ID]*kauripb.Node),
	}

	k.opts.SetShouldUseTree()
	k.eventLoop.RegisterHandler(hotstuff.ReplicaConnectedEvent{}, func(_ any) {
		k.postInit()
	}, core.Prioritize())
	k.eventLoop.RegisterHandler(ContributionRecvEvent{}, func(event any) {
		k.OnContributionRecv(event.(ContributionRecvEvent))
	})
	return k
}

func (k *Kauri) postInit() {
	k.logger.Info("Kauri: Initializing")
	kauripb.RegisterKauriServer(k.server.GetGorumsServer(), serviceImpl{k.eventLoop})
	k.initializeConfiguration()
}

func (k *Kauri) initializeConfiguration() {
	kauriCfg := kauripb.ConfigurationFromRaw(k.sender.GorumsConfig(), nil)
	for _, n := range kauriCfg.Nodes() {
		k.nodes[hotstuff.ID(n.ID())] = n
	}
	k.treeConfig = k.opts.TreeConfig()
	k.tree = *tree.CreateTree(k.opts.ID(), k.treeConfig.BranchFactor(), k.treeConfig.TreePos())
	k.initDone = true
	k.senders = make([]hotstuff.ID, 0)
}

// Begin starts dissemination of proposal and aggregation of votes.
func (k *Kauri) Begin(pc hotstuff.PartialCert, p hotstuff.ProposeMsg) {
	if !k.initDone {
		k.eventLoop.DelayUntil(sender.ConnectedEvent{}, func() { k.Begin(pc, p) })
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

func (k *Kauri) WaitToAggregate(waitTime time.Duration, view hotstuff.View) {
	time.Sleep(waitTime)
	if k.currentView != view {
		return
	}
	if !k.aggSent {
		k.SendContributionToParent()
		k.reset()
	}
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
		// delta is the network delay between two processes, at root it is 2*(3-1)*delta
		waitTime := time.Duration(2*(k.tree.TreeHeight()-1)) * k.treeConfig.TreeWaitDelta()
		go k.WaitToAggregate(waitTime, k.currentView)
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
				ID:        uint32(k.opts.ID()),
				Signature: hotstuffpb.QuorumSignatureToProto(k.aggContrib),
				View:      uint64(k.currentView),
			})
		}
	}
}

type serviceImpl struct {
	eventLoop *core.EventLoop
}

func (i serviceImpl) SendContribution(_ gorums.ServerCtx, request *kauripb.Contribution) {
	i.eventLoop.AddEvent(ContributionRecvEvent{Contribution: request})
}

// ContributionRecvEvent is raised when a contribution is received.
type ContributionRecvEvent struct {
	Contribution *kauripb.Contribution
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
	if !k.crypto.Verify(currentSignature, block.ToBytes()) {
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
	combSignature, err := k.crypto.Combine(currentSignature, k.aggContrib)
	if err != nil {
		return fmt.Errorf("failed to combine signatures: %v", err)
	}
	k.aggContrib = combSignature
	if combSignature.Participants().Len() >= k.configuration.QuorumSize() {
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
