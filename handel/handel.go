package handel

import (
	"context"
	"errors"
	"math"
	"sync"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/handelpb"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
)

type Handel struct {
	mut      sync.Mutex
	mods     *consensus.Modules
	cfg      *handelpb.Configuration
	maxLevel int
	sessions map[consensus.Hash]*session
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (h *Handel) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	h.mods = mods
}

func (h *Handel) Init() error {
	h.sessions = make(map[consensus.Hash]*session)

	var cfg *backend.Config
	var srv *backend.Server

	if !h.mods.GetModuleByType(&srv) {
		return errors.New("could not get gorums server")
	}
	if !h.mods.GetModuleByType(&cfg) {
		return errors.New("could not get gorums configuration")
	}

	handelpb.RegisterHandelServer(srv.GetGorumsServer(), serviceImpl{h})
	h.cfg = handelpb.ConfigurationFromRaw(cfg.GetRawConfiguration(), nil)

	h.maxLevel = int(math.Ceil(math.Log2(float64(h.mods.Configuration().Len()))))

	return nil
}

// Begin commissions the aggregation of a new signature.
func (h *Handel) Begin(ctx context.Context, s consensus.PartialCert) {
	// turn the single signature into a threshold signature,
	// this makes it easier to work with.
	ts := h.mods.Crypto().Combine(s)
	session := h.newSession(s.BlockHash(), ts)

	h.mut.Lock()
	h.sessions[s.BlockHash()] = session
	h.mut.Unlock()

	newCtx, cancel := context.WithCancel(ctx)

	go session.run(newCtx)

	go func() {
		select {
		case sig := <-session.done:
			cancel()
			h.mods.Synchronizer().AdvanceView(consensus.NewSyncInfo().WithQC(
				consensus.NewQuorumCert(sig, h.mods.Synchronizer().View(), session.hash),
			))
			h.mut.Lock()
			delete(h.sessions, session.hash)
			h.mut.Unlock()
		case <-ctx.Done():
		}
	}()
}

type serviceImpl struct {
	h *Handel
}

func (impl serviceImpl) Contribute(ctx gorums.ServerCtx, msg *handelpb.Contribution) {
	var hash consensus.Hash
	copy(hash[:], msg.GetHash())

	id, err := backend.GetPeerIDFromContext(ctx, impl.h.mods.Configuration())
	if err != nil {
		impl.h.mods.Logger().Error(err)
	}

	impl.h.mut.Lock()
	s, ok := impl.h.sessions[hash]
	impl.h.mut.Unlock()
	if !ok {
		impl.h.mods.Logger().Warnf("No session for block %v", hash)
		return
	}

	s.contributions <- contribution{
		sender:    id,
		level:     int(msg.GetLevel()),
		signature: hotstuffpb.ThresholdSignatureFromProto(msg.GetIndividual()),
	}

	s.contributions <- contribution{
		sender:    id,
		level:     int(msg.GetLevel()),
		signature: hotstuffpb.ThresholdSignatureFromProto(msg.GetAggregate()),
	}
}
