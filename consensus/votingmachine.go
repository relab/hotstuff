package consensus

import (
	"sync"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
)

// VotingMachine collects votes.
type VotingMachine struct {
	mut           sync.Mutex
	mods          *modules.ConsensusCore
	verifiedVotes map[hotstuff.Hash][]hotstuff.PartialCert // verified votes that could become a QC
}

// NewVotingMachine returns a new VotingMachine.
func NewVotingMachine() *VotingMachine {
	return &VotingMachine{
		verifiedVotes: make(map[hotstuff.Hash][]hotstuff.PartialCert),
	}
}

// InitConsensusModule gives the module a reference to the ConsensusCore object.
// It also allows the module to set module options using the OptionsBuilder.
func (vm *VotingMachine) InitConsensusModule(mods *modules.ConsensusCore, _ *modules.OptionsBuilder) {
	vm.mods = mods
	vm.mods.EventLoop().RegisterHandler(hotstuff.VoteMsg{}, func(event any) { vm.OnVote(event.(hotstuff.VoteMsg)) })
}

// OnVote handles an incoming vote.
func (vm *VotingMachine) OnVote(vote hotstuff.VoteMsg) {
	cert := vote.PartialCert
	vm.mods.Logger().Debugf("OnVote(%d): %.8s", vote.ID, cert.BlockHash())

	var (
		block *hotstuff.Block
		ok    bool
	)

	if !vote.Deferred {
		// first, try to get the block from the local cache
		block, ok = vm.mods.BlockChain().LocalGet(cert.BlockHash())
		if !ok {
			// if that does not work, we will try to handle this event later.
			// hopefully, the block has arrived by then.
			vm.mods.Logger().Debugf("Local cache miss for block: %.8s", cert.BlockHash())
			vote.Deferred = true
			vm.mods.EventLoop().DelayUntil(hotstuff.ProposeMsg{}, vote)
			return
		}
	} else {
		// if the block has not arrived at this point we will try to fetch it.
		block, ok = vm.mods.BlockChain().Get(cert.BlockHash())
		if !ok {
			vm.mods.Logger().Debugf("Could not find block for vote: %.8s.", cert.BlockHash())
			return
		}
	}

	if block.View() <= vm.mods.Synchronizer().LeafBlock().View() {
		// too old
		return
	}

	if vm.mods.Options().ShouldVerifyVotesSync() {
		vm.verifyCert(cert, block)
	} else {
		go vm.verifyCert(cert, block)
	}
}

func (vm *VotingMachine) verifyCert(cert hotstuff.PartialCert, block *hotstuff.Block) {
	if !vm.mods.Crypto().VerifyPartialCert(cert) {
		vm.mods.Logger().Info("OnVote: Vote could not be verified!")
		return
	}

	vm.mut.Lock()
	defer vm.mut.Unlock()

	// this defer will clean up any old votes in verifiedVotes
	defer func() {
		// delete any pending QCs with lower height than bLeaf
		for k := range vm.verifiedVotes {
			if block, ok := vm.mods.BlockChain().LocalGet(k); ok {
				if block.View() <= vm.mods.Synchronizer().LeafBlock().View() {
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

	if len(votes) < vm.mods.Configuration().QuorumSize() {
		return
	}

	qc, err := vm.mods.Crypto().CreateQuorumCert(block, votes)
	if err != nil {
		vm.mods.Logger().Info("OnVote: could not create QC for block: ", err)
		return
	}
	delete(vm.verifiedVotes, cert.BlockHash())

	vm.mods.EventLoop().AddEvent(hotstuff.NewViewMsg{ID: vm.mods.ID(), SyncInfo: hotstuff.NewSyncInfo().WithQC(qc)})
}
