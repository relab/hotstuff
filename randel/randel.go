package randel

import (
	"errors"
	"math"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/proto/randelpb"
	"github.com/relab/hotstuff/modules"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	modules.RegisterModule("randel", New)
}

// Handel implements a signature aggregation protocol.
type Randel struct {
	mods                    *consensus.Modules
	nodes                   map[hotstuff.ID]*randelpb.Node
	isContributionCollected bool
	aggregatedContribution  *randelpb.RContribution
	maxLevel                int
}

// New returns a new instance of the Handel module.
func New() consensus.Randel {
	return &Randel{
		nodes: make(map[hotstuff.ID]*randelpb.Node),
	}
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (r *Randel) InitConsensusModule(mods *consensus.Modules, opts *consensus.OptionsBuilder) {
	r.mods = mods
	opts.SetShouldUseHandel()

	// the rest of the setup is deferred to the Init method.
	// FIXME: it could be possible to handle the Init stuff automatically
	// if the Configuration module were to send an event upon connecting.
}

// Init initializes the Handel module.
func (r *Randel) Init() error {
	r.mods.Logger().Info("Handel: Initializing")

	var cfg *backend.Config
	var srv *backend.Server

	if !r.mods.GetModuleByType(&srv) {
		return errors.New("could not get gorums server")
	}
	if !r.mods.GetModuleByType(&cfg) {
		return errors.New("could not get gorums configuration")
	}

	randelpb.RegisterRandelServer(srv.GetGorumsServer(), serviceImpl{r})
	randelCfg := randelpb.ConfigurationFromRaw(cfg.GetRawConfiguration(), nil)

	for _, n := range randelCfg.Nodes() {
		r.nodes[hotstuff.ID(n.ID())] = n
	}

	r.maxLevel = int(math.Ceil(math.Log2(float64(r.mods.Configuration().Len()))))

	return nil
}

// Begin commissions the aggregation of a new signature.
func (h *Randel) Begin(s consensus.PartialCert) {
	level := h.assignLevel(s.BlockHash())
	var hash [32]byte = s.BlockHash()
	sig := hotstuffpb.QuorumSignatureToProto(s.Signature())
	h.aggregatedContribution = &randelpb.RContribution{
		ID:        uint32(h.mods.ID()),
		Signature: sig,
		Hash:      hash[:],
	}
	for temp := 1; temp < level.getLevel(h.mods.ID()); temp++ {
		subNodes := level.getSubNodesForNode(temp, h.mods.ID())
		for _, id := range subNodes {
			node := h.nodes[id]
		}
	}
}

type serviceImpl struct {
	r *Randel
}

func (impl serviceImpl) RequestContribution(ctx gorums.ServerCtx,
	in *emptypb.Empty) (resp *randelpb.RContribution, err error) {
	// TODO(Hanish): It is a quorum function for handling the
	// response from the sub configuration.
	// Verify the contribution and send the response to the above level.
	waitTime := 10 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return impl.r.aggregatedContribution, ctx.Err()
		case <-time.After(waitTime):
			if impl.r.isContributionCollected == true {
				return impl.r.aggregatedContribution, nil
			}
		}
	}
	return nil, errors.New("Unable to collect the signature")
}

type contribution struct {
	hash       consensus.Hash
	sender     hotstuff.ID
	level      int
	signature  consensus.QuorumSignature
	individual consensus.QuorumSignature
	verified   bool
	deferred   bool
	score      int
}

type disseminateEvent struct {
	sessionID consensus.Hash
}

type levelActivateEvent struct {
	sessionID consensus.Hash
}

type sessionDoneEvent struct {
	hash consensus.Hash
}
