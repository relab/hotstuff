package randel

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/proto/randelpb"
	"github.com/relab/hotstuff/modules"
	"golang.org/x/net/context"
)

func init() {
	modules.RegisterModule("randel", New)
}

// Handel implements a signature aggregation protocol.
type Randel struct {
	sync.Mutex
	mods                    *consensus.Modules
	nodes                   map[hotstuff.ID]*randelpb.Node
	initialTimeoutMilliSecs int
	maxLevel                int
	aggregatedContribution  *randelpb.RContribution
	individualContribution  *randelpb.RContribution
	level                   *Level
	prevFailedNodeIDs       []hotstuff.ID
	newFailedNodeIDs        []hotstuff.ID
	blockHash               []byte
	isAggregationCompleted  bool
	isInitialized           bool
}

// New returns a new instance of the Handel module.
func New() consensus.Randel {
	return &Randel{
		nodes:             make(map[hotstuff.ID]*randelpb.Node),
		prevFailedNodeIDs: make([]hotstuff.ID, 0),
	}
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (r *Randel) InitConsensusModule(mods *consensus.Modules, opts *consensus.OptionsBuilder) {
	r.mods = mods
	opts.SetShouldUseRandel()
}

// Init initializes the Handel module.
func (r *Randel) Init() error {
	r.mods.Logger().Info("Randel: Initializing")

	var cfg *backend.Config
	var srv *backend.Server

	if !r.mods.GetModuleByType(&srv) {
		return errors.New("could not get gorums server")
	}
	if !r.mods.GetModuleByType(&cfg) {
		return errors.New("could not get gorums configuration")
	}

	randelCfg := randelpb.ConfigurationFromRaw(cfg.GetRawConfiguration(), nil)

	for _, n := range randelCfg.Nodes() {
		r.nodes[hotstuff.ID(n.ID())] = n
	}
	r.level = r.assignLevel()
	r.maxLevel = int(math.Ceil(math.Log2(float64(r.mods.Configuration().Len()))))
	r.initialTimeoutMilliSecs = 2000
	randelpb.RegisterRandelServer(srv.GetGorumsServer(), serviceImpl{r})
	return nil
}

// Begin commissions the aggregation of a new signature.
func (r *Randel) Begin(s consensus.PartialCert) {

	r.Lock()
	defer r.Unlock()
	r.mods.Logger().Info("Randel: object initializing")
	r.reset()
	var hash [32]byte = s.BlockHash()
	copy(r.blockHash, hash[:])
	sig := hotstuffpb.QuorumSignatureToProto(s.Signature())
	r.individualContribution = &randelpb.RContribution{
		ID:        uint32(r.mods.ID()),
		Signature: sig,
		Hash:      hash[:],
	}
	r.mods.Logger().Info("My level is ", r.level.getLevel(r.mods.ID()))
	if r.level.getLevel(r.mods.ID()) == 1 || r.level.getLevel(r.mods.ID()) == 0 {
		r.aggregatedContribution = r.individualContribution
	}
	r.isInitialized = true
	r.mods.Logger().Info("Initialization completed")
	go r.fetchContributions()
}

func (r *Randel) reset() {
	r.prevFailedNodeIDs = make([]hotstuff.ID, 0)
	r.newFailedNodeIDs = make([]hotstuff.ID, 0)
	r.blockHash = make([]byte, 32)
	r.aggregatedContribution = nil
	r.individualContribution = nil
	r.isAggregationCompleted = false
	r.isInitialized = false
}

func (r *Randel) canMergeContributions(a, b consensus.QuorumSignature) bool {
	canMerge := true
	if a == nil || b == nil {
		r.mods.Logger().Info("one of it is nil")
		return false
	}
	a.Participants().RangeWhile(func(i hotstuff.ID) bool {
		b.Participants().RangeWhile(func(j hotstuff.ID) bool {
			// cannot merge a and b if they both contain a contribution from the same ID.
			if i == j {
				r.mods.Logger().Info("one of it is same")
				canMerge = false
			}

			return canMerge
		})

		return canMerge
	})

	return canMerge
}

func (r *Randel) verifyContribution(signature consensus.QuorumSignature, hash consensus.Hash) bool {
	verified := false
	block, ok := r.mods.BlockChain().Get(hash)
	if !ok {
		return verified
	}
	verified = r.mods.Crypto().Verify(signature, block.ToBytes())
	return verified
}
func (r *Randel) mergeWithContribution(contribution *randelpb.RContribution) (bool, error) {
	r.Lock()
	defer r.Unlock()
	if r.aggregatedContribution == nil {
		r.aggregatedContribution = contribution
		return false, nil
	}
	currentSignature := hotstuffpb.QuorumSignatureFromProto(contribution.Signature)
	compiledSignature := hotstuffpb.QuorumSignatureFromProto(r.aggregatedContribution.Signature)
	var blockHash consensus.Hash
	copy(blockHash[:], r.blockHash)
	isVerified := r.verifyContribution(currentSignature, blockHash)
	if !isVerified {
		r.mods.Logger().Info("contribution verification failed ")
		return false, errors.New("unable to verify the contribution")
	}
	if r.canMergeContributions(currentSignature, compiledSignature) {
		new, err := r.mods.Crypto().Combine(currentSignature, compiledSignature)
		if err == nil {
			r.mods.Logger().Info("combination done with length ", new.Participants().Len())
			r.aggregatedContribution.Signature = hotstuffpb.QuorumSignatureToProto(new)
			if new.Participants().Len() >= r.mods.Configuration().QuorumSize() {
				r.mods.Logger().Info("sending the event to loop ")
				var blockHash consensus.Hash
				copy(blockHash[:], r.blockHash)
				r.mods.EventLoop().AddEvent(consensus.NewViewMsg{
					SyncInfo: consensus.NewSyncInfo().WithQC(consensus.NewQuorumCert(
						new,
						r.mods.Synchronizer().View(),
						blockHash,
					)),
				})
				return true, nil
			}
		} else {
			r.mods.Logger().Info("Failed to combine signatures: %v", err)
			return false, errors.New("unable to combine signature")
		}
	} else {
		r.mods.Logger().Info("Failed to merge signatures")
		return false, errors.New("unable to merge signature")
	}
	return false, nil
}

func (r *Randel) fetchContributionFromNode(nodeID hotstuff.ID, level int, isAldFailed bool) {
	node := r.nodes[nodeID]
	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration((level*r.initialTimeoutMilliSecs))*time.Millisecond)
	defer cancel()
	contribution, err := node.RequestContribution(ctx, &randelpb.Request{NodeID: uint32(r.mods.ID()), View: uint64(r.mods.Synchronizer().View())})
	if err != nil {
		r.mods.Logger().Info("unable to fetch contribution from node:", nodeID, err)
		r.newFailedNodeIDs = append(r.newFailedNodeIDs, nodeID)
		return
	}
	if contribution.Signature == nil {
		return
	}

	r.mergeWithContribution(contribution)
	// TODO(hanish): We are assuming no incorrect contribution for implementation.
	if !isAldFailed {
		for _, fID := range contribution.FailedNodes {
			if hotstuff.ID(fID) == r.mods.ID() {
				continue
			}
			r.Lock()
			r.prevFailedNodeIDs = append(r.prevFailedNodeIDs, hotstuff.ID(fID))
			r.Unlock()
		}
	}
}

func (r *Randel) fetchContributions() {

	subNodes := r.level.getAllSubNodes(r.mods.ID())
	r.mods.Logger().Info("My sub nodes ", subNodes)

	for _, nodeID := range subNodes {
		r.fetchContributionFromNode(nodeID, r.level.getLevel(r.mods.ID()), false)
	}
	r.fetchFromFailedSubNodes()
	r.Lock()
	r.isAggregationCompleted = true
	r.Unlock()
	r.mods.Logger().Info("Completed the fetching ")
}

func (r *Randel) fetchFromFailedSubNodes() {
	r.Lock()
	defer r.Unlock()
	myLevel := r.level.getLevel(r.mods.ID())
	for _, nodeID := range r.prevFailedNodeIDs {
		r.fetchContributionFromNode(nodeID, myLevel, true)
	}
}

type serviceImpl struct {
	r *Randel
}

func (impl serviceImpl) sendContribution(request *randelpb.Request) (resp *randelpb.RContribution, err error) {
	impl.r.Lock()
	defer impl.r.Unlock()
	if impl.r.isInitialized {
		if impl.r.mods.Synchronizer().View() < consensus.View(request.View) {

			impl.r.mods.Logger().Info("i am slow ", impl.r.mods.Synchronizer().View(), consensus.View(request.View))
			return nil, errors.New("not updated yet")
		}
		if impl.r.mods.Synchronizer().View() > consensus.View(request.View) {
			emptyContribution := &randelpb.RContribution{}
			return emptyContribution, nil
		}
		myLevel := impl.r.level.getLevel(impl.r.mods.ID())
		reqLevel := impl.r.level.getLevel(hotstuff.ID(request.NodeID))
		if myLevel == 0 {
			impl.r.mods.Logger().Info("lower request received")
			return impl.r.individualContribution, nil
		}
		if myLevel > reqLevel {
			impl.r.mods.Logger().Info("lower request received")
			return impl.r.individualContribution, nil
		}
		if impl.r.isAggregationCompleted {
			if impl.r.aggregatedContribution == nil {
				if impl.r.individualContribution != nil {
					impl.r.mods.Logger().Info("sending the individual contribution")
					return impl.r.individualContribution, nil
				}
				return nil, errors.New("unable to collect the signatures")
			}
			for _, nodeID := range impl.r.newFailedNodeIDs {
				impl.r.aggregatedContribution.FailedNodes = append(impl.r.aggregatedContribution.FailedNodes, uint32(nodeID))
			}
			impl.r.mods.Logger().Info("sending the aggregate contribution")
			return impl.r.aggregatedContribution, nil
		}
	}
	return nil, errors.New("not initialized yet")
}
func (impl serviceImpl) RequestContribution(ctx gorums.ServerCtx,
	request *randelpb.Request) (resp *randelpb.RContribution, err error) {
	emptyContribution := &randelpb.RContribution{}
	impl.r.mods.Logger().Info("received contribution request")

	for {
		impl.r.mods.Logger().Info("waiting for select ")
		select {
		case <-ctx.Done():
			impl.r.mods.Logger().Info("context is complete, sending the aggregate contribution")
			if impl.r.aggregatedContribution == nil {
				return emptyContribution, ctx.Err()
			}
			return impl.r.aggregatedContribution, ctx.Err()
		default:
			contribution, err := impl.sendContribution(request)
			if err != nil {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			return contribution, err
		}
	}
}
