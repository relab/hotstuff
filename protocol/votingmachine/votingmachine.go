package votingmachine

import (
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
)

// VotingMachine collects and verifies votes.
// TODO(AlanRostem): make tests
type VotingMachine struct {
	logger     logging.Logger
	eventLoop  *eventloop.EventLoop
	config     *core.RuntimeConfig
	blockchain *blockchain.Blockchain
	auth       *cert.Authority
	state      *protocol.ViewStates

	mut           sync.Mutex
	verifiedVotes map[hotstuff.Hash][]hotstuff.PartialCert
}

func New(
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	auth *cert.Authority,
	state *protocol.ViewStates,
) *VotingMachine {
	vm := &VotingMachine{
		blockchain:    blockchain,
		auth:          auth,
		eventLoop:     eventLoop,
		logger:        logger,
		config:        config,
		state:         state,
		verifiedVotes: make(map[hotstuff.Hash][]hotstuff.PartialCert),
	}
	vm.eventLoop.RegisterHandler(hotstuff.VoteMsg{}, func(event any) {
		vm.CollectVote(event.(hotstuff.VoteMsg))
	})
	return vm
}

// CollectVote handles an incoming vote.
func (vm *VotingMachine) CollectVote(vote hotstuff.VoteMsg) {
	cert := vote.PartialCert
	vm.logger.Debugf("CollectVote(from %d): %s", vote.ID, cert.BlockHash().SmallString())
	var (
		block *hotstuff.Block
		ok    bool
	)
	if !vote.Deferred {
		// first, try to get the block from the local cache
		block, ok = vm.blockchain.LocalGet(cert.BlockHash())
		if !ok {
			// if that does not work, we will try to handle this event later.
			// hopefully, the block has arrived by then.
			vm.logger.Debugf("Local cache miss for block: %s", cert.BlockHash().SmallString())
			vote.Deferred = true
			vm.eventLoop.DelayUntil(hotstuff.ProposeMsg{}, vote)
			return
		}
	} else {
		// if the block has not arrived at this point we will try to fetch it.
		block, ok = vm.blockchain.Get(cert.BlockHash())
		if !ok {
			vm.logger.Debugf("Could not find block for vote: %s.", cert.BlockHash().SmallString())
			return
		}
	}
	if block.View() <= vm.state.HighQC().View() {
		vm.logger.Info("block too old")
		return
	}
	if vm.config.SyncVerification() {
		vm.verifyCert(cert, block)
	} else {
		go vm.verifyCert(cert, block)
	}
}

func (vm *VotingMachine) verifyCert(cert hotstuff.PartialCert, block *hotstuff.Block) {
	if err := vm.auth.VerifyPartialCert(cert); err != nil {
		vm.logger.Info("vote could not be verified: %v", err)
		return
	}
	vm.mut.Lock()
	defer vm.mut.Unlock()
	// this defer will clean up any old votes in verifiedVotes
	defer func() {
		// delete any pending QCs with lower height than bLeaf
		for k := range vm.verifiedVotes {
			if block, ok := vm.blockchain.LocalGet(k); ok {
				if block.View() <= vm.state.HighQC().View() {
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
		vm.logger.Infof("CollectVote: could not create QC for block: %v", err)
		return
	}
	delete(vm.verifiedVotes, cert.BlockHash())
	vm.logger.Debug("CollectVote: dispatching event for new view (current : %d)", vm.state.View())
	vm.eventLoop.AddEvent(hotstuff.NewViewMsg{ID: vm.config.ID(), SyncInfo: hotstuff.NewSyncInfo().WithQC(qc)})
}
