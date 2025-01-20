package consensus

import (
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
)

// VotingMachine collects votes.
type VotingMachine struct {
	comps core.ComponentList

	mut           sync.Mutex
	verifiedVotes map[hotstuff.Hash][]hotstuff.PartialCert // verified votes that could become a QC
}

// NewVotingMachine returns a new VotingMachine.
func NewVotingMachine() *VotingMachine {
	return &VotingMachine{
		verifiedVotes: make(map[hotstuff.Hash][]hotstuff.PartialCert),
	}
}

// InitComponent initializes the VotingMachine.
func (vm *VotingMachine) InitComponent(mods *core.Core) {
	vm.comps = mods.Components()
	vm.comps.EventLoop.RegisterHandler(hotstuff.VoteMsg{}, func(event any) { vm.OnVote(event.(hotstuff.VoteMsg)) })
}

// OnVote handles an incoming vote.
func (vm *VotingMachine) OnVote(vote hotstuff.VoteMsg) {
	cert := vote.PartialCert
	vm.comps.Logger.Debugf("OnVote(%d): %.8s", vote.ID, cert.BlockHash())

	var (
		block *hotstuff.Block
		ok    bool
	)

	if !vote.Deferred {
		// first, try to get the block from the local cache
		block, ok = vm.comps.BlockChain.LocalGet(cert.BlockHash())
		if !ok {
			// if that does not work, we will try to handle this event later.
			// hopefully, the block has arrived by then.
			vm.comps.Logger.Debugf("Local cache miss for block: %.8s", cert.BlockHash())
			vote.Deferred = true
			vm.comps.EventLoop.DelayUntil(hotstuff.ProposeMsg{}, vote)
			return
		}
	} else {
		// if the block has not arrived at this point we will try to fetch it.
		block, ok = vm.comps.BlockChain.Get(cert.BlockHash())
		if !ok {
			vm.comps.Logger.Debugf("Could not find block for vote: %.8s.", cert.BlockHash())
			return
		}
	}

	if block.View() <= vm.comps.Synchronizer.HighQC().View() {
		// too old
		return
	}

	if vm.comps.Options.ShouldVerifyVotesSync() {
		vm.verifyCert(cert, block)
	} else {
		go vm.verifyCert(cert, block)
	}
}

func (vm *VotingMachine) verifyCert(cert hotstuff.PartialCert, block *hotstuff.Block) {
	if !vm.comps.Crypto.VerifyPartialCert(cert) {
		vm.comps.Logger.Info("OnVote: Vote could not be verified!")
		return
	}

	vm.mut.Lock()
	defer vm.mut.Unlock()

	// this defer will clean up any old votes in verifiedVotes
	defer func() {
		// delete any pending QCs with lower height than bLeaf
		for k := range vm.verifiedVotes {
			if block, ok := vm.comps.BlockChain.LocalGet(k); ok {
				if block.View() <= vm.comps.Synchronizer.HighQC().View() {
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

	if len(votes) < vm.comps.Configuration.QuorumSize() {
		return
	}

	qc, err := vm.comps.Crypto.CreateQuorumCert(block, votes)
	if err != nil {
		vm.comps.Logger.Info("OnVote: could not create QC for block: ", err)
		return
	}
	delete(vm.verifiedVotes, cert.BlockHash())

	vm.comps.EventLoop.AddEvent(hotstuff.NewViewMsg{ID: vm.comps.Options.ID(), SyncInfo: hotstuff.NewSyncInfo().WithQC(qc)})
}
