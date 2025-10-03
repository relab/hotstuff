package comm

import (
	"fmt"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol/comm/kauri"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
)

const NameKauri = "kauri"

// Kauri implements tree-based dissemination and aggregation.
type Kauri struct {
	logger     logging.Logger
	eventLoop  *eventloop.EventLoop
	config     *core.RuntimeConfig
	blockchain *blockchain.Blockchain
	auth       *cert.Authority
	sender     core.KauriSender

	aggContrib  hotstuff.QuorumSignature
	aggSent     bool
	blockHash   hotstuff.Hash
	currentView hotstuff.View
	initDone    bool
	senders     []hotstuff.ID
	tree        *tree.Tree
}

// NewKauri creates a new Kauri instance for communicating proposals and votes.
func NewKauri(
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	auth *cert.Authority,
	sender core.KauriSender,
) *Kauri {
	if !config.HasKauriTree() {
		panic("the tree must be configured for kauri")
	}
	k := &Kauri{
		blockchain: blockchain,
		auth:       auth,
		config:     config,
		eventLoop:  eventLoop,
		sender:     sender,
		logger:     logger,

		senders: make([]hotstuff.ID, 0),
		tree:    config.Tree(),
	}
	k.eventLoop.RegisterHandler(hotstuff.ReplicaConnectedEvent{}, func(_ any) {
		k.initDone = true // this signals that
	})
	k.eventLoop.RegisterHandler(kauri.ContributionRecvEvent{}, func(event any) {
		k.OnContributionRecv(event.(kauri.ContributionRecvEvent))
	})
	k.eventLoop.RegisterHandler(WaitTimerExpiredEvent{}, func(event any) {
		k.OnWaitTimerExpired(event.(WaitTimerExpiredEvent))
	})
	return k
}

// Begin starts dissemination of proposal and aggregation of votes.
func (k *Kauri) Begin(p *hotstuff.ProposeMsg, pc hotstuff.PartialCert) error {
	if !k.initDone {
		k.eventLoop.DelayUntil(network.ConnectedEvent{}, func() {
			if err := k.Begin(p, pc); err != nil {
				k.logger.Error(err)
			}
		})
		return nil
	}
	k.reset()
	k.blockHash = pc.BlockHash()
	k.currentView = p.Block.View()
	k.aggContrib = pc.Signature()
	return k.SendProposalToChildren(p)
}

func (k *Kauri) reset() {
	k.aggContrib = nil
	k.senders = make([]hotstuff.ID, 0)
	k.aggSent = false
}

func (k *Kauri) Aggregate(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert) error {
	return k.Begin(proposal, pc)
}

func (k *Kauri) Disseminate(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert) error {
	return k.Begin(proposal, pc)
}

func (k *Kauri) WaitToAggregate() {
	view := k.currentView
	time.Sleep(k.tree.WaitTime())
	k.eventLoop.AddEvent(WaitTimerExpiredEvent{currentView: view})
}

// SendProposalToChildren sends the proposal to the children.
func (k *Kauri) SendProposalToChildren(p *hotstuff.ProposeMsg) error {
	children := k.tree.ReplicaChildren()
	if len(children) != 0 {
		childSender, err := k.sender.Sub(children)
		if err != nil {
			return fmt.Errorf("unable to send the proposal to children: %w", err)
		}
		k.logger.Debug("Sending proposal to children ", children)
		childSender.Propose(p)
		go k.WaitToAggregate()
	} else {
		k.sender.SendContributionToParent(k.currentView, k.aggContrib)
		k.aggSent = true
	}
	return nil
}

// OnContributionRecv is invoked upon receiving the vote for aggregation.
func (k *Kauri) OnContributionRecv(event kauri.ContributionRecvEvent) {
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
	if kauri.IsSubSet(k.tree.SubTree(), k.senders) {
		k.sender.SendContributionToParent(k.currentView, k.aggContrib)
		k.aggSent = true
	}
}

func (k *Kauri) OnWaitTimerExpired(event WaitTimerExpiredEvent) {
	if k.currentView != event.currentView {
		return
	}
	if !k.aggSent {
		k.sender.SendContributionToParent(k.currentView, k.aggContrib)
		k.reset()
	}
}

func (k *Kauri) mergeContribution(currentSignature hotstuff.QuorumSignature) error {
	block, ok := k.blockchain.Get(k.blockHash)
	if !ok {
		return fmt.Errorf("failed to fetch block %s", k.blockHash.SmallString())
	}
	if err := k.auth.Verify(currentSignature, block.ToBytes()); err != nil {
		return err
	}
	if k.aggContrib == nil {
		// first contribution
		k.aggContrib = currentSignature
		return nil
	}
	if err := kauri.CanMergeContributions(currentSignature, k.aggContrib); err != nil {
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

type WaitTimerExpiredEvent struct {
	currentView hotstuff.View
}

var _ Communication = (*Kauri)(nil)
