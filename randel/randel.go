package randel

import (
	"context"
	"encoding/binary"
	"errors"
	"math/rand"
	"reflect"
	"sort"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/proto/randelpb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("randel", New)
}

// Handel implements a signature aggregation protocol.
type Randel struct {
	//sync.Mutex
	configuration          *backend.Config
	server                 *backend.Server
	blockChain             modules.BlockChain
	crypto                 modules.Crypto
	eventLoop              *eventloop.EventLoop
	logger                 logging.Logger
	opts                   *modules.Options
	leaderRotation         modules.LeaderRotation
	nodes                  map[hotstuff.ID]*randelpb.Node
	tree                   *TreeConfiguration
	initDone               bool
	beginDone              bool
	aggregatedContribution hotstuff.QuorumSignature
	isAggregationCompleted bool
	cancelFunc             context.CancelFunc
	blockHash              hotstuff.Hash
	currentView            hotstuff.View
	ProposalMsg            hotstuff.ProposeMsg
	children               []hotstuff.ID
	senders                []hotstuff.ID
}

// New returns a new instance of the Handel module.
func New() modules.Randel {
	return &Randel{
		nodes:   make(map[hotstuff.ID]*randelpb.Node),
		senders: make([]hotstuff.ID, 0),
	}
}

func (r *Randel) InitModule(mods *modules.Core) {
	mods.Get(
		&r.configuration,
		&r.server,
		&r.blockChain,
		&r.crypto,
		&r.eventLoop,
		&r.logger,
		&r.opts,
		&r.leaderRotation,
	)
	r.opts.SetShouldUseRandel()
	r.eventLoop.RegisterObserver(backend.ConnectedEvent{}, func(_ any) {
		r.postInit()
	})
	// register handler for contribution and timeout event
	r.eventLoop.RegisterHandler(ContributionRecvEvent{}, func(event any) {
		r.OnContributionRecv(event.(ContributionRecvEvent))
	})
	r.eventLoop.RegisterHandler(NACKRecvEvent{}, func(event any) {
		r.OnNACK(event.(NACKRecvEvent))
	})
}

func (r *Randel) cancelWaitingContext() {
	if r.cancelFunc != nil {
		r.cancelFunc()
		r.cancelFunc = nil
	}
}

func (r *Randel) OnContributionRecv(event ContributionRecvEvent) {

	contribution := event.Contribution
	r.logger.Info("processing the contribution from ", contribution.ID)
	// if contribution.View > uint64(r.currentView) {
	// 	r.logger.Info("waiting for the propose")
	// 	r.eventLoop.DelayUntil(hotstuff.ProposeMsg{}, event)
	// 	return
	// }
	if r.isAggregationCompleted {
		return
	}
	currentSignature := hotstuffpb.QuorumSignatureFromProto(contribution.Signature)
	isDone, err := r.mergeWithContribution(currentSignature)
	if err != nil {
		return
	}
	r.senders = append(r.senders, hotstuff.ID(contribution.ID))
	if isSubSet(r.children, r.senders) || isDone {
		pID, ok := r.tree.GetParent()
		if ok {
			r.SendContributionToNode(pID, r.aggregatedContribution)
		}
		r.sendACKToSenders()
		r.isAggregationCompleted = true
		r.cancelWaitingContext()
	} else {
		participants := currentSignature.Participants()
		nodeChildren := r.tree.GetChildrenOfNode(hotstuff.ID(contribution.ID))
		if participants.Len() != len(nodeChildren) {
			failedNodes := make([]hotstuff.ID, 0)
			for _, ID := range nodeChildren {
				if !participants.Contains(ID) {
					failedNodes = append(failedNodes, ID)
				}
			}
			r.sendNACKToFailedNodes(failedNodes)
		}
	}
}

func (r *Randel) OnNACK(event NACKRecvEvent) {
	gpID, ok := r.tree.GetGrandParent()
	if ok {
		r.SendContributionToNode(gpID, r.aggregatedContribution)
	}
}

func (r *Randel) postInit() {
	r.logger.Info("Randel: Initializing")
	randelCfg := randelpb.ConfigurationFromRaw(r.configuration.GetRawConfiguration(), nil)
	for _, n := range randelCfg.Nodes() {
		r.nodes[hotstuff.ID(n.ID())] = n
	}
	randelpb.RegisterRandelServer(r.server.GetGorumsServer(), serviceImpl{r})
	r.tree = CreateTree(r.configuration.Len(), r.opts.ID())
	r.initDone = true
}

func (r *Randel) Begin(s hotstuff.PartialCert, p hotstuff.ProposeMsg, v hotstuff.View) {
	if !r.initDone {
		// wait until initialization is done
		r.eventLoop.DelayUntil(backend.ConnectedEvent{}, func() { r.Begin(s, p, v) })
		return
	}

	r.reset()
	r.currentView = v
	r.beginDone = true
	r.blockHash = s.BlockHash()
	r.ProposalMsg = p
	// sig := hotstuffpb.QuorumSignatureToProto(s.Signature())
	// individualContribution := &randelpb.RContribution{
	// 	ID:        uint32(r.opts.ID()),
	// 	Signature: sig,
	// 	Hash:      r.blockHash[:],
	// 	View:      uint64(v),
	// }
	r.aggregatedContribution = s.Signature()

	// idMappings := make(map[hotstuff.ID]int)
	// for i := 0; i < r.configuration.Len(); i++ {
	// 	idMappings[hotstuff.ID(i+1)] = i
	// }
	idMappings := r.randomizeIDS(r.blockHash, r.leaderRotation.GetLeader(r.currentView))
	r.tree.InitializeWithPIDs(idMappings)
	r.children = r.tree.GetChildren()
	r.sendProposalToChildren(p, s.Signature())
}

func (r *Randel) sendNACKToFailedNodes(failedNodes []hotstuff.ID) {
	if len(failedNodes) == 0 {
		return
	} else {
		for _, nodeID := range failedNodes {
			node, ok := r.nodes[nodeID]
			if !ok {
				r.logger.Error("node not found in map ", nodeID, r.nodes)
				continue
			}
			r.logger.Info("sending SecondChance from ", r.opts.ID(), " to ",
				nodeID, " for view ", r.currentView)
			proposal := hotstuffpb.ProposalToProto(r.ProposalMsg)
			node.SendNoAck(context.Background(), &randelpb.NACK{NodeID: uint32(nodeID),
				Proposal: proposal})
		}
	}
}

func (r *Randel) sendACKToSenders() {
	if len(r.senders) == 0 || r.aggregatedContribution == nil {
		return
	} else {
		for _, nodeID := range r.senders {
			node, ok := r.nodes[nodeID]
			if !ok {
				r.logger.Error("node not found in map ", nodeID, r.nodes)
				continue
			}
			contribution := randelpb.RContribution{
				ID:        uint32(r.tree.ID),
				Signature: hotstuffpb.QuorumSignatureToProto(r.aggregatedContribution),
				View:      uint64(r.currentView),
			}
			r.logger.Info("sending acknowledgement from ", r.opts.ID(), " to ",
				nodeID, " for view ", contribution.View)
			node.SendAcknowledgement(context.Background(), &contribution)
		}
	}
}

func (r *Randel) sendProposalToChildren(proposal hotstuff.ProposeMsg, individual hotstuff.QuorumSignature) {

	if len(r.children) == 0 {
		parent, ok := r.tree.GetParent()
		if ok {
			r.SendContributionToNode(parent, individual)
		}
	} else {
		config, err := r.configuration.SubConfig(r.children)
		if err != nil {
			r.logger.Error("Unable to send the proposal to children", err)
			return
		}
		config.Propose(proposal)
		timeout := time.Duration(3)
		context, cancel := context.WithTimeout(context.Background(), timeout*time.Millisecond)
		r.cancelFunc = cancel
		go r.waitForContributions(context, r.currentView)
	}
}

func (r *Randel) waitForContributions(ctx context.Context, view hotstuff.View) {

	<-ctx.Done()
	if r.currentView != view {
		return
	}
	if !r.isAggregationCompleted {
		pID, ok := r.tree.GetParent()
		if ok {
			r.logger.Info("sending contribution due to timeout", pID)
			r.SendContributionToNode(pID, r.aggregatedContribution)
		}
	}
}

func (r *Randel) SendContributionToNode(nodeID hotstuff.ID, quorumSignature hotstuff.QuorumSignature) {
	emptyContribution := &randelpb.RContribution{}
	node, ok := r.nodes[nodeID]
	if !ok {
		r.logger.Error("node not found in map ", nodeID, r.nodes)
		return
	}
	if quorumSignature == nil {
		node.SendContribution(context.Background(), emptyContribution)
	} else {
		contribution := randelpb.RContribution{
			ID:        uint32(r.tree.ID),
			Signature: hotstuffpb.QuorumSignatureToProto(quorumSignature),
			View:      uint64(r.currentView),
		}
		r.logger.Info("sending contribution from ", r.opts.ID(), " to ", nodeID, " for view ", contribution.View)
		node.SendContribution(context.Background(), &contribution)
	}
}

func (r *Randel) reset() {
	r.beginDone = false
	r.aggregatedContribution = nil
	r.isAggregationCompleted = false
	r.senders = make([]hotstuff.ID, 0)
	r.cancelWaitingContext()
}

func (r *Randel) canMergeContributions(a, b hotstuff.QuorumSignature) bool {
	canMerge := true
	if a == nil || b == nil {
		r.logger.Info("one of it is nil")
		return false
	}
	a.Participants().RangeWhile(func(i hotstuff.ID) bool {
		b.Participants().RangeWhile(func(j hotstuff.ID) bool {
			// cannot merge a and b if they both contain a contribution from the same ID.
			if i == j {
				r.logger.Info("one of it is same", i)
				canMerge = false
			}

			return canMerge
		})

		return canMerge
	})

	return canMerge
}

func (r *Randel) verifyContribution(signature hotstuff.QuorumSignature, hash hotstuff.Hash) bool {
	verified := false
	block, ok := r.blockChain.Get(hash)
	if !ok {
		return verified
	}
	verified = r.crypto.Verify(signature, block.ToBytes())
	return verified
}
func (r *Randel) mergeWithContribution(currentSignature hotstuff.QuorumSignature) (bool, error) {

	isVerified := r.verifyContribution(currentSignature, r.blockHash)
	if !isVerified {
		r.logger.Info("Contribution verification failed")
		return false, errors.New("unable to verify the contribution")
	}
	if r.aggregatedContribution == nil {
		r.aggregatedContribution = currentSignature
		r.logger.Info("Contribution set  initially")
		return false, nil
	}

	//compiledSignature := hotstuffpb.QuorumSignatureFromProto(r.aggregatedContribution.Signature)
	if r.canMergeContributions(currentSignature, r.aggregatedContribution) {
		new, err := r.crypto.Combine(currentSignature, r.aggregatedContribution)
		if err == nil {
			r.logger.Info("combination done with length ", new.Participants().Len())
			r.aggregatedContribution = new
			if new.Participants().Len() >= r.configuration.QuorumSize() {
				r.logger.Info("sending the event to loop ")
				r.eventLoop.AddEvent(hotstuff.NewViewMsg{
					SyncInfo: hotstuff.NewSyncInfo().WithQC(hotstuff.NewQuorumCert(
						r.aggregatedContribution,
						r.currentView,
						r.blockHash,
					)),
				})
				return true, nil
			}
		} else {
			r.logger.Info("Failed to combine signatures: %v", err)
			return false, errors.New("unable to combine signature")
		}
	} else {
		r.logger.Info("Failed to merge signatures")
		return false, errors.New("unable to merge signature")
	}
	return false, nil
}

type serviceImpl struct {
	r *Randel
}

func (i serviceImpl) SendAcknowledgement(ctx gorums.ServerCtx, request *randelpb.RContribution) {
	i.r.logger.Info("Received acknowledgment, storing the acknowledgement")
	//TODO(hanish): Should we check the aggregation Signature before assigning?
	if request.View == uint64(i.r.currentView) {
		i.r.aggregatedContribution = hotstuffpb.QuorumSignatureFromProto(request.Signature)
	}
}

func (i serviceImpl) SendNoAck(ctx gorums.ServerCtx, request *randelpb.NACK) {
	i.r.logger.Info("Received NACK from node ", request.NodeID)
	proposal := request.Proposal
	proposal.Block.Proposer = uint32(request.NodeID)
	proposeMsg := hotstuffpb.ProposalFromProto(proposal)
	proposeMsg.ID = hotstuff.ID(request.NodeID)
	i.r.eventLoop.AddEvent(proposeMsg)
}

func (i serviceImpl) SendContribution(ctx gorums.ServerCtx, request *randelpb.RContribution) {

	if request.View >= uint64(i.r.currentView) {
		i.r.eventLoop.AddEvent(ContributionRecvEvent{Contribution: request})
	} else {
		i.r.logger.Info("Received contribution for older view ", request.View)
	}
}

type ContributionRecvEvent struct {
	Contribution *randelpb.RContribution
}

type NACKRecvEvent struct {
	View hotstuff.View
}

type InitCompleteEvent struct {
	View hotstuff.View
}

func (r *Randel) randomizeIDS(hash hotstuff.Hash, leaderID hotstuff.ID) map[hotstuff.ID]int {
	//assign leader to the root of the tree.
	seed := r.opts.SharedRandomSeed() + int64(binary.LittleEndian.Uint64(hash[:]))
	totalNodes := r.configuration.Len()
	ids := make([]hotstuff.ID, 0, totalNodes)
	for id := range r.configuration.Replicas() {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	// Shuffle the list of IDs using the shared random seed + the first 8 bytes of the hash.
	rnd := rand.New(rand.NewSource(seed))
	rnd.Shuffle(len(ids), reflect.Swapper(ids))
	lIndex := 0
	for index, id := range ids {
		if id == leaderID {
			lIndex = index
		}
	}
	currentRoot := ids[0]
	ids[0] = ids[lIndex]
	ids[lIndex] = currentRoot
	posMapping := make(map[hotstuff.ID]int)
	for index, ID := range ids {
		posMapping[ID] = index
	}
	return posMapping
}

// check if a is subset of b
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
