// Package handel provides signature aggregation through a peer-to-peer protocol.
// This could help reduce the load on the leader in very large configurations of replicas.
// This package implements the `Handel` interface defined in the consensus package.
// The following gives an overview of how this module works:
//
// Enabling Handel from the CLI.
//
// Handel can be enabled through the `--modules` flag:
//
//	./hotstuff run --modules="handel"
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
	"math"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/proto/handelpb"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

func init() {
	modules.RegisterModule("handel", New)
}

// Handel implements a signature aggregation protocol.
type Handel struct {
	configuration *backend.Config
	server        *backend.Server

	blockChain   modules.BlockChain
	crypto       modules.Crypto
	eventLoop    *eventloop.EventLoop
	logger       logging.Logger
	opts         *modules.Options
	synchronizer modules.Synchronizer

	nodes    map[hotstuff.ID]*handelpb.Node
	maxLevel int
	sessions map[hotstuff.Hash]*session
	initDone bool
}

// New returns a new instance of the Handel module.
func New() modules.Handel {
	return &Handel{
		nodes:    make(map[hotstuff.ID]*handelpb.Node),
		sessions: make(map[hotstuff.Hash]*session),
	}
}

// InitModule initializes the Handel module.
func (h *Handel) InitModule(mods *modules.Core) {
	mods.Get(
		&h.configuration,
		&h.server,

		&h.blockChain,
		&h.crypto,
		&h.eventLoop,
		&h.logger,
		&h.opts,
		&h.synchronizer,
	)

	h.opts.SetShouldUseHandel()

	h.eventLoop.RegisterObserver(backend.ConnectedEvent{}, func(_ any) {
		h.postInit()
	})

	h.eventLoop.RegisterHandler(contribution{}, func(event any) {
		c := event.(contribution)
		if s, ok := h.sessions[c.hash]; ok {
			s.handleContribution(c)
		} else if !c.deferred {
			c.deferred = true
			h.eventLoop.DelayUntil(hotstuff.ProposeMsg{}, c)
		}
	})

	h.eventLoop.RegisterHandler(sessionDoneEvent{}, func(event any) {
		e := event.(sessionDoneEvent)
		delete(h.sessions, e.hash)
	})
}

func (h *Handel) postInit() {
	h.logger.Info("Handel: Initializing")

	h.maxLevel = int(math.Ceil(math.Log2(float64(h.configuration.Len()))))

	handelCfg := handelpb.ConfigurationFromRaw(h.configuration.GetRawConfiguration(), nil)
	for _, n := range handelCfg.Nodes() {
		h.nodes[hotstuff.ID(n.ID())] = n
	}

	handelpb.RegisterHandelServer(h.server.GetGorumsServer(), serviceImpl{h})

	// now we can start handling timer events
	h.eventLoop.RegisterHandler(disseminateEvent{}, func(e any) {
		ctx, cancel := synchronizer.ViewContext(h.eventLoop.Context(), h.eventLoop, nil)
		defer cancel()
		if s, ok := h.sessions[e.(disseminateEvent).sessionID]; ok {
			s.sendContributions(ctx)
		}
	})

	h.eventLoop.RegisterHandler(levelActivateEvent{}, func(e any) {
		if s, ok := h.sessions[e.(levelActivateEvent).sessionID]; ok {
			s.advanceLevel()
		}
	})

	h.initDone = true
}

// Begin commissions the aggregation of a new signature.
func (h *Handel) Begin(s hotstuff.PartialCert) {
	if !h.initDone {
		// wait until initialization is done
		h.eventLoop.DelayUntil(backend.ConnectedEvent{}, func() { h.Begin(s) })
		return
	}

	// turn the single signature into a threshold signature,
	// this makes it easier to work with.
	session := h.newSession(s.BlockHash(), s.Signature())
	h.sessions[s.BlockHash()] = session

	go session.verifyContributions()
}

type serviceImpl struct {
	h *Handel
}

func (impl serviceImpl) Contribute(ctx gorums.ServerCtx, msg *handelpb.Contribution) {
	var hash hotstuff.Hash
	copy(hash[:], msg.GetHash())

	id, err := backend.GetPeerIDFromContext(ctx, impl.h.configuration)
	if err != nil {
		impl.h.logger.Error(err)
	}

	sig := hotstuffpb.QuorumSignatureFromProto(msg.GetSignature())
	indiv := hotstuffpb.QuorumSignatureFromProto(msg.GetIndividual())

	if sig != nil && indiv != nil {
		impl.h.eventLoop.AddEvent(contribution{
			hash:       hash,
			sender:     id,
			level:      int(msg.GetLevel()),
			signature:  sig,
			individual: indiv,
			verified:   false,
		})
	} else {
		impl.h.logger.Warnf("contribution received with invalid signatures: %v, %v", sig, indiv)
	}
}

type contribution struct {
	hash       hotstuff.Hash
	sender     hotstuff.ID
	level      int
	signature  hotstuff.QuorumSignature
	individual hotstuff.QuorumSignature
	verified   bool
	deferred   bool
	score      int
}

type disseminateEvent struct {
	sessionID hotstuff.Hash
}

type levelActivateEvent struct {
	sessionID hotstuff.Hash
}

type sessionDoneEvent struct {
	hash hotstuff.Hash
}
