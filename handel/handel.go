package handel

import (
	"context"
	"errors"
	"math"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/handelpb"
)

type Handel struct {
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

}

type serviceImpl struct {
	h *Handel
}

func (impl serviceImpl) Contribute(ctx gorums.ServerCtx, msg *handelpb.Contribution) {

}
