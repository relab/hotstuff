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
func (vm *Leader) CollectVote(vote hotstuff.VoteMsg) {
	cert := vote.PartialCert
	vm.logger.Debugf("OnVote(%d): %.8s", vote.ID, cert.BlockHash())

	var (
		block *hotstuff.Block
		ok    bool
	)

	if !vote.Deferred {
		// first, try to get the block from the local cache
		block, ok = vm.blockChain.LocalGet(cert.BlockHash())
		if !ok {
			// if that does not work, we will try to handle this event later.
			// hopefully, the block has arrived by then.
			vm.logger.Debugf("Local cache miss for block: %.8s", cert.BlockHash())
			vote.Deferred = true
			vm.eventLoop.DelayUntil(hotstuff.ProposeMsg{}, vote)
			return
		}
	} else {
		// if the block has not arrived at this point we will try to fetch it.
		block, ok = vm.blockChain.Get(cert.BlockHash())
		if !ok {
			vm.logger.Debugf("Could not find block for vote: %.8s.", cert.BlockHash())
			return
		}
	}

	if block.View() <= vm.auth.HighQC().View() {
		// too old
		return
	}

	if vm.config.SyncVoteVerification() {
		vm.verifyCert(cert, block)
	} else {
		go vm.verifyCert(cert, block)
	}
}

func (vm *Leader) verifyCert(cert hotstuff.PartialCert, block *hotstuff.Block) {
	if !vm.auth.VerifyPartialCert(cert) {
		vm.logger.Info("OnVote: Vote could not be verified!")
		return
	}

	vm.mut.Lock()
	defer vm.mut.Unlock()

	// this defer will clean up any old votes in verifiedVotes
	defer func() {
		// delete any pending QCs with lower height than bLeaf
		for k := range vm.verifiedVotes {
			if block, ok := vm.blockChain.LocalGet(k); ok {
				if block.View() <= vm.auth.HighQC().View() {
					delete(vm.verifiedVotes, k)
				}
			} else {
				delete(vm.verifiedVotes, k)
			}
		}
	}()

	votes := vm.verifiedVotes[cert.BlockHash()]
	votes = append(votes, cert)
	vm.verifiedVotes[cert.BlockHash()] = votes

	if len(votes) < vm.config.QuorumSize() {
		return
	}

	qc, err := vm.auth.CreateQuorumCert(block, votes)
	if err != nil {
		vm.logger.Info("OnVote: could not create QC for block: ", err)
		return
	}
	delete(vm.verifiedVotes, cert.BlockHash())

	vm.eventLoop.AddEvent(hotstuff.NewViewMsg{ID: vm.config.ID(), SyncInfo: hotstuff.NewSyncInfo().WithQC(qc)})
}

func Propose(_ hotstuff.ProposeMsg) {
	// TODO(AlanRostem): put Consensus.Propose in here.
}
