package randel

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
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
	fmt.Print("registered randel")
}

// Handel implements a signature aggregation protocol.
type Randel struct {
	sync.Mutex
	configuration          *backend.Config
	server                 *backend.Server
	blockChain             modules.BlockChain
	crypto                 modules.Crypto
	eventLoop              *eventloop.EventLoop
	logger                 logging.Logger
	opts                   *modules.Options
	leaderRotation         modules.LeaderRotation
	nodes                  map[hotstuff.ID]*randelpb.Node
	level                  *Level
	maxLevel               int
	initDone               bool
	beginDone              bool
	individualContribution *randelpb.RContribution
	aggregatedContribution *randelpb.RContribution
	gotSubNodes            []hotstuff.ID
	isAggregationCompleted bool
	cancelFunc             context.CancelFunc
	blockHash              hotstuff.Hash
	currentView            hotstuff.View
}

// New returns a new instance of the Handel module.
func New() modules.Randel {
	return &Randel{
		nodes: make(map[hotstuff.ID]*randelpb.Node),
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

func (r *Randel) OnContributionRecv(event ContributionRecvEvent) {
	contribution := event.Contribution
	r.Lock()
	defer r.Unlock()
	if contribution == nil {
		r.gotSubNodes = append(r.gotSubNodes, hotstuff.ID(0))
		return
	}
	r.logger.Info("processing the contribution from ", contribution.ID)

	if contribution.View > uint64(r.currentView) {
		r.logger.Info("waiting for the propose")
		r.eventLoop.DelayUntil(hotstuff.ProposeMsg{}, event)
		return
	}
	if !r.beginDone {
		r.logger.Info("waiting for the init complete")
		r.eventLoop.DelayUntil(InitCompleteEvent{}, event)
		return
	}

	isDone, err := r.mergeWithContribution(contribution)
	if err != nil {
		return
	}
	r.gotSubNodes = append(r.gotSubNodes, hotstuff.ID(contribution.ID))
	for _, failNodeID := range contribution.FailedNodes {
		rNode := r.nodes[hotstuff.ID(failNodeID)]
		r.logger.Info("Sending nack to ", failNodeID)
		rNode.SendNoAck(context.Background(), &randelpb.Request{NodeID: uint32(r.opts.ID()),
			View: contribution.View})
	}
	if len(r.level.GetChildren()) == len(r.gotSubNodes) && r.level.GetLevel() != r.maxLevel {
		lID := r.level.GetParent()
		r.SendContributionToNode(lID, r.aggregatedContribution)
		r.isAggregationCompleted = true
		r.cancelFunc()
	}
	if isDone {
		r.logger.Info("aggregation completed")
		r.isAggregationCompleted = true
		r.cancelFunc()
	}
}

func (r *Randel) OnNACK(event NACKRecvEvent) {
	r.Lock()
	defer r.Unlock()
	if r.level.GetLevel() == r.maxLevel {
		return
	}
	gpID := r.level.GetGrandParent()
	r.SendContributionToNode(gpID, r.aggregatedContribution)
}

func (r *Randel) postInit() {
	r.logger.Info("Randel: Initializing")

	r.maxLevel = int(math.Ceil(math.Log2(float64(r.configuration.Len()))))

	randelCfg := randelpb.ConfigurationFromRaw(r.configuration.GetRawConfiguration(), nil)
	for _, n := range randelCfg.Nodes() {
		r.nodes[hotstuff.ID(n.ID())] = n
	}
	randelpb.RegisterRandelServer(r.server.GetGorumsServer(), serviceImpl{r})
	r.level = CreateLevelMapping(r.configuration.Len(), r.opts.ID())
	r.initDone = true
}

func (r *Randel) Begin(s hotstuff.PartialCert, p hotstuff.ProposeMsg, v hotstuff.View) {
	if !r.initDone {
		// wait until initialization is done
		r.eventLoop.DelayUntil(backend.ConnectedEvent{}, func() { r.Begin(s, p, v) })
		return
	}
	r.Lock()
	defer r.Unlock()
	r.reset()
	r.currentView = v
	r.beginDone = true
	r.blockHash = s.BlockHash()
	sig := hotstuffpb.QuorumSignatureToProto(s.Signature())
	r.individualContribution = &randelpb.RContribution{
		ID:        uint32(r.opts.ID()),
		Signature: sig,
		Hash:      r.blockHash[:],
		View:      uint64(v),
	}
	//idMappings := r.randomizeIDS(hash, r.leaderRotation.GetLeader(r.synchronizer.View()))
	idMappings := make(map[hotstuff.ID]int)
	for i := 0; i < r.configuration.Len(); i++ {
		idMappings[hotstuff.ID(i+1)] = i
	}
	r.level.InitializeWithPIDs(idMappings)
	r.logger.Info("Randel: init done")
	if r.level.GetLevel() == 0 {
		r.SendContributionToNode(r.level.GetLevel1Peer(), r.individualContribution)
		return
	} else if r.level.GetLevel() != 1 {
		r.sendProposalToChildren(p)
		r.SendContributionToNode(r.level.GetLevel1Peer(), r.individualContribution)
		if r.level.GetLevel() == r.maxLevel &&
			r.leaderRotation.GetLeader(r.currentView) == r.opts.ID() {
			r.sendProposalToZeroLevelNodes(p)
		}
	} else {
		r.aggregatedContribution = r.individualContribution
	}
	r.eventLoop.AddEvent(InitCompleteEvent{View: hotstuff.View(r.currentView)})
	timeout := time.Duration(r.level.GetLevel() * 500)
	context, cancel := context.WithTimeout(context.Background(), timeout*time.Millisecond)
	r.cancelFunc = cancel
	go r.waitForContributions(context, r.currentView)
}

func (r *Randel) sendProposalToChildren(proposal hotstuff.ProposeMsg) {
	config, err := r.configuration.SubConfig(r.level.GetChildren())
	if err != nil {
		r.logger.Error("Unable to send the proposal to children", err)
		return
	}
	config.Propose(proposal)
}

func (r *Randel) sendProposalToZeroLevelNodes(proposal hotstuff.ProposeMsg) {
	config, err := r.configuration.SubConfig(r.level.GetZeroLevelReplicas())
	if err != nil {
		r.logger.Error("Unable to send the proposal to children", err)
		return
	}
	config.Propose(proposal)
}
func (r *Randel) waitForContributions(ctx context.Context, view hotstuff.View) {

	// <-ctx.Done()
	// if r.currentView != view {
	// 	return
	// }
	// if !r.isAggregationCompleted {
	// 	lID := r.level.GetParent()
	// 	r.logger.Info("sending contribution due to timeout", lID)
	// 	r.SendContributionToNode(lID, r.aggregatedContribution)
	// }
}

func (r *Randel) SendContributionToNode(nodeID hotstuff.ID, contribution *randelpb.RContribution) {
	emptyContribution := &randelpb.RContribution{}
	if nodeID == r.opts.ID() {
		r.eventLoop.AddEvent(ContributionRecvEvent{Contribution: contribution})
		return
	}
	node, ok := r.nodes[nodeID]
	if !ok {
		r.logger.Error("node not found in map ", nodeID, r.nodes)
		return
	}
	if contribution == nil {
		node.SendContribution(context.Background(), emptyContribution)
	} else {
		contribution.View = uint64(r.currentView)
		contribution.ID = uint32(r.opts.ID())
		r.logger.Info("sending contribution from ", r.opts.ID(), " to ", nodeID, " for view ", contribution.View)
		node.SendContribution(context.Background(), contribution)
	}
}

func (r *Randel) reset() {
	r.beginDone = false
	r.individualContribution = nil
	r.aggregatedContribution = nil
	r.gotSubNodes = make([]hotstuff.ID, 0)
	r.isAggregationCompleted = false
	if r.cancelFunc != nil {
		r.cancelFunc()
	}
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
func (r *Randel) mergeWithContribution(contribution *randelpb.RContribution) (bool, error) {

	currentSignature := hotstuffpb.QuorumSignatureFromProto(contribution.Signature)
	var blockHash hotstuff.Hash
	copy(blockHash[:], contribution.Hash[:])
	isVerified := r.verifyContribution(currentSignature, blockHash)
	if !isVerified {
		r.logger.Info("Contribution verification failed")
		return false, errors.New("unable to verify the contribution")
	}
	if r.aggregatedContribution == nil {
		r.aggregatedContribution = contribution
		r.logger.Info("Contribution set  initially")
		return false, nil
	}

	compiledSignature := hotstuffpb.QuorumSignatureFromProto(r.aggregatedContribution.Signature)
	if r.canMergeContributions(currentSignature, compiledSignature) {
		new, err := r.crypto.Combine(currentSignature, compiledSignature)
		if err == nil {
			r.logger.Info("combination done with length ", new.Participants().Len())
			r.aggregatedContribution.Signature = hotstuffpb.QuorumSignatureToProto(new)
			if new.Participants().Len() >= r.configuration.QuorumSize() {
				r.logger.Info("sending the event to loop ")
				var blockHash hotstuff.Hash
				copy(blockHash[:], r.blockHash[:])
				r.eventLoop.AddEvent(hotstuff.NewViewMsg{
					SyncInfo: hotstuff.NewSyncInfo().WithQC(hotstuff.NewQuorumCert(
						new,
						r.currentView,
						blockHash,
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
	i.r.logger.Info("Recevied acknowledement, storing the acknowledgement")
	//TODO(hanish): Should we check the aggregation Signature before assigning?
	if request.View == uint64(i.r.currentView) {
		var hash [32]byte
		copy(hash[:], request.Hash[:])
		i.r.aggregatedContribution = &randelpb.RContribution{
			ID:        uint32(i.r.opts.ID()),
			Signature: request.Signature,
			Hash:      hash[:],
		}
		hotstuffpb.QuorumSignatureFromProto(request.Signature)
	}

}

func (i serviceImpl) SendNoAck(ctx gorums.ServerCtx, request *randelpb.Request) {
	i.r.Lock()
	defer i.r.Unlock()
	i.r.logger.Info("Received NACK from node ", request.NodeID)
	if request.View == uint64(i.r.currentView) {
		i.r.eventLoop.AddEvent(NACKRecvEvent{View: hotstuff.View(request.View)})
	}
}

func (i serviceImpl) SendContribution(ctx gorums.ServerCtx, request *randelpb.RContribution) {
	i.r.Lock()
	defer i.r.Unlock()
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
