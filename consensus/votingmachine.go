package consensus

import (
	"fmt"
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

// VotingMachine collects votes.
type VotingMachine struct {
	blockChain    modules.BlockChain
	configuration modules.Configuration
	crypto        modules.Crypto
	eventLoop     *eventloop.ScopedEventLoop
	logger        logging.Logger
	synchronizer  modules.Synchronizer
	opts          *modules.Options

	pipe          hotstuff.Pipe
	mut           sync.Mutex
	verifiedVotes map[hotstuff.Hash][]hotstuff.PartialCert // verified votes that could become a QC
}

// NewVotingMachine returns a new VotingMachine.
func NewVotingMachine() *VotingMachine {
	return &VotingMachine{
		verifiedVotes: make(map[hotstuff.Hash][]hotstuff.PartialCert),
	}
}

// InitModule initializes the VotingMachine.
func (vm *VotingMachine) InitModule(mods *modules.Core, info modules.ScopeInfo) {
	mods.GetScoped(vm,
		&vm.blockChain,
		&vm.configuration,
		&vm.crypto,
		&vm.eventLoop,
		&vm.logger,
		&vm.synchronizer,
		&vm.opts,
	)

	vm.pipe = info.ModuleScope
	vm.eventLoop.RegisterHandler(hotstuff.VoteMsg{}, func(event any) {
		vm.OnVote(event.(hotstuff.VoteMsg))
	}, eventloop.RespondToScope(info.ModuleScope))
}

// OnVote handles an incoming vote.
func (vm *VotingMachine) OnVote(vote hotstuff.VoteMsg) {
	cert := vote.PartialCert
	if vm.pipe != cert.Pipe() {
		panic("incorrect pipe")
	}

	vm.logger.Debugf("OnVote[p=%d, view=%d](vote=%d): %.8s", vm.pipe, vm.synchronizer.View(), vote.ID, cert.BlockHash())

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
			vm.logger.Debugf("Local cache miss for block [p=%d, view=%d]: %.8s", vm.pipe, vm.synchronizer.View(), cert.BlockHash())
			vote.Deferred = true
			vm.eventLoop.DelayScoped(vm.pipe, hotstuff.ProposeMsg{}, vote)
			return
		}
	} else {
		// if the block has not arrived at this point we will try to fetch it.
		block, ok = vm.blockChain.Get(cert.BlockHash(), cert.Pipe())
		if !ok {
			vm.logger.Debugf("Could not find block for vote [p=%d, view=%d]", vm.pipe, vm.synchronizer.View())
			return
		}
	}

	if block.View() <= vm.synchronizer.HighQC().View() {
		// too old
		return
	}

	if vm.opts.ShouldVerifyVotesSync() {
		vm.verifyCert(cert, block)
	} else {
		go vm.verifyCert(cert, block)
	}
}

func (vm *VotingMachine) verifyCert(cert hotstuff.PartialCert, block *hotstuff.Block) {
	if !vm.crypto.VerifyPartialCert(cert) {
		vm.logger.Infof("OnVote[p=%d, view=%d]: Vote could not be verified!", vm.pipe, vm.synchronizer.View())
		return
	}

	vm.mut.Lock()
	defer vm.mut.Unlock()

	// this defer will clean up any old votes in verifiedVotes
	defer func() {
		// delete any pending QCs with lower height than bLeaf
		for k := range vm.verifiedVotes {
			if block, ok := vm.blockChain.LocalGet(k); ok {
				if block.View() <= vm.synchronizer.HighQC().View() {
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

	if len(votes) < vm.configuration.QuorumSize() {
		return
	}

	qc, err := vm.crypto.CreateQuorumCert(block, votes)
	if err != nil {
		vm.logger.Info(fmt.Sprintf("OnVote[p=%d, view=%d]: could not create QC for block: ", vm.pipe, vm.synchronizer.View()), err)
		return
	}
	delete(vm.verifiedVotes, cert.BlockHash())

	vm.eventLoop.AddScopedEvent(vm.pipe, hotstuff.NewViewMsg{
		ID:       vm.opts.ID(),
		SyncInfo: hotstuff.NewSyncInfo(block.Pipe()).WithQC(qc)})
}
