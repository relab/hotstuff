package consensus

import (
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
)

type Leader struct {
	logger     logging.Logger
	eventLoop  *eventloop.EventLoop
	config     *core.RuntimeConfig
	blockChain *blockchain.BlockChain
	auth       *certauth.CertAuthority

	mut           sync.Mutex
	verifiedVotes map[hotstuff.Hash][]hotstuff.PartialCert
}

func NewLeader(
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	blockChain *blockchain.BlockChain,
	auth *certauth.CertAuthority,
) *Leader {
	ld := &Leader{
		blockChain:    blockChain,
		auth:          auth,
		eventLoop:     eventLoop,
		logger:        logger,
		config:        config,
		verifiedVotes: make(map[hotstuff.Hash][]hotstuff.PartialCert),
	}
	ld.eventLoop.RegisterHandler(hotstuff.VoteMsg{}, func(event any) {
		ld.CollectVote(event.(hotstuff.VoteMsg))
	})
	return ld
}

// CollectVote handles an incoming vote.
func (ld *Leader) CollectVote(vote hotstuff.VoteMsg) {
	cert := vote.PartialCert
	ld.logger.Debugf("OnVote(%d): %.8s", vote.ID, cert.BlockHash())

	var (
		block *hotstuff.Block
		ok    bool
	)

	if !vote.Deferred {
		// first, try to get the block from the local cache
		block, ok = ld.blockChain.LocalGet(cert.BlockHash())
		if !ok {
			// if that does not work, we will try to handle this event later.
			// hopefully, the block has arrived by then.
			ld.logger.Debugf("Local cache miss for block: %.8s", cert.BlockHash())
			vote.Deferred = true
			ld.eventLoop.DelayUntil(hotstuff.ProposeMsg{}, vote)
			return
		}
	} else {
		// if the block has not arrived at this point we will try to fetch it.
		block, ok = ld.blockChain.Get(cert.BlockHash())
		if !ok {
			ld.logger.Debugf("Could not find block for vote: %.8s.", cert.BlockHash())
			return
		}
	}

	if block.View() <= ld.auth.HighQC().View() {
		// too old
		return
	}

	if ld.config.SyncVoteVerification() {
		ld.verifyCert(cert, block)
	} else {
		go ld.verifyCert(cert, block)
	}
}

func (ld *Leader) verifyCert(cert hotstuff.PartialCert, block *hotstuff.Block) {
	if !ld.auth.VerifyPartialCert(cert) {
		ld.logger.Info("OnVote: Vote could not be verified!")
		return
	}

	ld.mut.Lock()
	defer ld.mut.Unlock()

	// this defer will clean up any old votes in verifiedVotes
	defer func() {
		// delete any pending QCs with lower height than bLeaf
		for k := range ld.verifiedVotes {
			if block, ok := ld.blockChain.LocalGet(k); ok {
				if block.View() <= ld.auth.HighQC().View() {
					delete(ld.verifiedVotes, k)
				}
			} else {
				delete(ld.verifiedVotes, k)
			}
		}
	}()

	votes := ld.verifiedVotes[cert.BlockHash()]
	votes = append(votes, cert)
	ld.verifiedVotes[cert.BlockHash()] = votes

	if len(votes) < ld.config.QuorumSize() {
		return
	}

	qc, err := ld.auth.CreateQuorumCert(block, votes)
	if err != nil {
		ld.logger.Info("OnVote: could not create QC for block: ", err)
		return
	}
	delete(ld.verifiedVotes, cert.BlockHash())

	ld.eventLoop.AddEvent(hotstuff.NewViewMsg{ID: ld.config.ID(), SyncInfo: hotstuff.NewSyncInfo().WithQC(qc)})
}

func Propose(_ hotstuff.ProposeMsg) {
	// TODO(AlanRostem): put Consensus.Propose in here.
}
