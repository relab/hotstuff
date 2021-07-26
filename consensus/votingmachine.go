package consensus

import (
	"sync"
)

// VotingMachine collects votes.
type VotingMachine struct {
	mut           sync.Mutex
	mod           *Modules
	verifiedVotes map[Hash][]PartialCert // verified votes that could become a QC
}

// NewVotingMachine returns a new VotingMachine.
func NewVotingMachine() *VotingMachine {
	return &VotingMachine{
		verifiedVotes: make(map[Hash][]PartialCert),
	}
}

// InitModule gives the module a reference to the HotStuff object. It also allows the module to set configuration
// settings using the ConfigBuilder.
func (vm *VotingMachine) InitModule(hs *Modules, _ *OptionsBuilder) {
	vm.mod = hs
	vm.mod.EventLoop().RegisterAsyncHandler(VoteMsg{}, func(event interface{}) {
		vote := event.(VoteMsg)
		go vm.OnVote(vote)
	})
}

// OnVote handles an incoming vote.
func (vm *VotingMachine) OnVote(vote VoteMsg) {
	cert := vote.PartialCert
	vm.mod.Logger().Debugf("OnVote(%d): %.8s", vote.ID, cert.BlockHash())

	var (
		block *Block
		ok    bool
	)

	if !vote.Deferred {
		// first, try to get the block from the local cache
		block, ok = vm.mod.BlockChain().LocalGet(cert.BlockHash())
		if !ok {
			// if that does not work, we will try to handle this event later.
			// hopefully, the block has arrived by then.
			vm.mod.Logger().Debugf("Local cache miss for block: %.8s", cert.BlockHash())
			vote.Deferred = true
			vm.mod.EventLoop().DelayUntil(ProposeMsg{}, vote)
			return
		}
	} else {
		// if the block has not arrived at this point we will try to fetch it.
		block, ok = vm.mod.BlockChain().Get(cert.BlockHash())
		if !ok {
			vm.mod.Logger().Debugf("Could not find block for vote: %.8s.", cert.BlockHash())
			return
		}
	}

	if block.View() <= vm.mod.Synchronizer().LeafBlock().View() {
		// too old
		return
	}

	if !vm.mod.Crypto().VerifyPartialCert(cert) {
		vm.mod.Logger().Info("OnVote: Vote could not be verified!")
		return
	}

	vm.mut.Lock()
	defer vm.mut.Unlock()

	// this defer will clean up any old votes in verifiedVotes
	defer func() {
		// delete any pending QCs with lower height than bLeaf
		for k := range vm.verifiedVotes {
			if block, ok := vm.mod.BlockChain().LocalGet(k); ok {
				if block.View() <= vm.mod.Synchronizer().LeafBlock().View() {
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

	if len(votes) < vm.mod.Configuration().QuorumSize() {
		return
	}

	qc, err := vm.mod.Crypto().CreateQuorumCert(block, votes)
	if err != nil {
		vm.mod.Logger().Info("OnVote: could not create QC for block: ", err)
		return
	}
	delete(vm.verifiedVotes, cert.BlockHash())

	// signal the synchronizer
	// because votes are handled asynchronously, we can safely use AddEvent without starting a goroutine.
	vm.mod.EventLoop().AddEvent(NewViewMsg{ID: vm.mod.ID(), SyncInfo: NewSyncInfo().WithQC(qc)})
}
