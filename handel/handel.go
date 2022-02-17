package handel

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/handelpb"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("handel", New)
}

// Handel implements a signature aggregation protocol.
type Handel struct {
	mut      sync.Mutex
	mods     *consensus.Modules
	cfg      *handelpb.Configuration
	maxLevel int
	sessions map[consensus.Hash]*session
}

// New returns a new instance of the Handel module.
func New() consensus.Handel {
	return &Handel{}
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (h *Handel) InitConsensusModule(mods *consensus.Modules, opts *consensus.OptionsBuilder) {
	h.mods = mods
	opts.SetShouldUseHandel()

	// the rest of the setup is deferred to the Init method.
	// FIXME: it could be possible to handle the Init stuff automatically
	// if the Configuration module were to send an event upon connecting.
}

// Init initializes the Handel module.
func (h *Handel) Init() error {
	h.mods.Logger().Info("Handel: Initializing")

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

	h.mods.EventLoop().RegisterHandler(contribution{}, func(event interface{}) {
		c := event.(contribution)

		if s, ok := h.sessions[c.hash]; ok {
			s.handleContribution(c)
		}
	})

	h.mods.EventLoop().RegisterHandler(disseminateEvent{}, func(_ interface{}) {
		for _, s := range h.sessions {
			s.sendContributions(s.h.mods.Synchronizer().ViewContext())
		}
	})

	h.mods.EventLoop().RegisterHandler(levelActivateEvent{}, func(_ interface{}) {
		for _, s := range h.sessions {
			s.activateLevel()
		}
	})

	h.mods.EventLoop().RegisterHandler(sessionDoneEvent{}, func(event interface{}) {
		e := event.(sessionDoneEvent)
		delete(h.sessions, e.hash)
	})

	h.mods.EventLoop().AddTicker(disseminationPeriod, func(_ time.Time) (_ interface{}) {
		return disseminateEvent{}
	})

	h.mods.EventLoop().AddTicker(levelActivateInterval, func(_ time.Time) (_ interface{}) {
		return levelActivateEvent{}
	})

	return nil
}

// Begin commissions the aggregation of a new signature.
func (h *Handel) Begin(s consensus.PartialCert) {
	// turn the single signature into a threshold signature,
	// this makes it easier to work with.
	ts := h.mods.Crypto().Combine(s.Signature())

	session := h.newSession(s.BlockHash(), ts)
	h.sessions[s.BlockHash()] = session

	go session.verifyContributions(h.mods.Synchronizer().ViewContext())
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

	if sig := hotstuffpb.ThresholdSignatureFromProto(msg.GetIndividual()); sig != nil {
		impl.h.mods.EventLoop().AddEvent(contribution{
			hash:      hash,
			sender:    id,
			level:     int(msg.GetLevel()),
			signature: sig,
			verified:  false,
		})
	}

	if sig := hotstuffpb.ThresholdSignatureFromProto(msg.GetAggregate()); sig != nil {
		impl.h.mods.EventLoop().AddEvent(contribution{
			hash:      hash,
			sender:    id,
			level:     int(msg.GetLevel()),
			signature: sig,
			verified:  false,
		})
	}
}

type disseminateEvent struct{}

type levelActivateEvent struct{}

type sessionDoneEvent struct {
	hash consensus.Hash
}
