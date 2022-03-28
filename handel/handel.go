// Package handel provides signature aggregation through a peer-to-peer protocol.
// This could help reduce the load on the leader in very large configurations of replicas.
// This package implements the `Handel` interface defined in the consensus package.
// The following gives an overview of how this module works:
//
// Enabling Handel from the CLI.
//
// Handel can be enabled through the `--modules` flag:
//  ./hotstuff run --modules="handel"
//
// Initialization.
//
// Handel requires an extra initialization step, because it needs to access the `Configuration`
// module after it has connected to the other replicas. In order to facilitate this, the
// orchestration runtime will check if a Handel module is present, after the Configuration has
// been connected, and then call the `Init` function of the Handel module.
//
// API.
//
// The Handel module exports only the `Begin` method. This method is used to start the aggregation process.
// When called, the `Begin` method will create a new Handel session. The Handel module forwards relevant
// events from the event loop to the active sessions.
//
// When a session has managed to assemble a valid quorum certificate, it will send a NewView event on the
// event loop.
package handel

import (
	"errors"
	"math"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
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
	mods     *consensus.Modules
	nodes    map[hotstuff.ID]*handelpb.Node
	maxLevel int
	sessions map[consensus.Hash]*session
}

// New returns a new instance of the Handel module.
func New() consensus.Handel {
	return &Handel{
		nodes: make(map[hotstuff.ID]*handelpb.Node),
	}
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
	handelCfg := handelpb.ConfigurationFromRaw(cfg.GetRawConfiguration(), nil)

	for _, n := range handelCfg.Nodes() {
		h.nodes[hotstuff.ID(n.ID())] = n
	}

	h.maxLevel = int(math.Ceil(math.Log2(float64(h.mods.Configuration().Len()))))

	h.mods.EventLoop().RegisterHandler(contribution{}, func(event interface{}) {
		c := event.(contribution)
		if s, ok := h.sessions[c.hash]; ok {
			s.handleContribution(c)
		} else if !c.deferred {
			c.deferred = true
			h.mods.EventLoop().DelayUntil(consensus.ProposeMsg{}, c)
		}
	})

	h.mods.EventLoop().RegisterHandler(disseminateEvent{}, func(e interface{}) {
		if s, ok := h.sessions[e.(disseminateEvent).sessionID]; ok {
			s.sendContributions(s.h.mods.Synchronizer().ViewContext())
		}
	})

	h.mods.EventLoop().RegisterHandler(levelActivateEvent{}, func(e interface{}) {
		if s, ok := h.sessions[e.(levelActivateEvent).sessionID]; ok {
			s.advanceLevel()
		}
	})

	h.mods.EventLoop().RegisterHandler(sessionDoneEvent{}, func(event interface{}) {
		e := event.(sessionDoneEvent)
		delete(h.sessions, e.hash)
	})

	return nil
}

// Begin commissions the aggregation of a new signature.
func (h *Handel) Begin(s consensus.PartialCert) {
	// turn the single signature into a threshold signature,
	// this makes it easier to work with.
	session := h.newSession(s.BlockHash(), s.Signature())
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

	sig := hotstuffpb.QuorumSignatureFromProto(msg.GetSignature())
	indiv := hotstuffpb.QuorumSignatureFromProto(msg.GetIndividual())

	if sig != nil && indiv != nil {
		impl.h.mods.EventLoop().AddEvent(contribution{
			hash:       hash,
			sender:     id,
			level:      int(msg.GetLevel()),
			signature:  sig,
			individual: indiv,
			verified:   false,
		})
	} else {
		impl.h.mods.Logger().Warnf("contribution received with invalid signatures: %v, %v", sig, indiv)
	}
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
