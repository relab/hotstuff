package chainedhotstuff

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/crypto/ecdsa"
)

// Builder is used to set up a HotStuff instance
type Builder struct {
	Config       hotstuff.Config
	BlockChain   hotstuff.BlockChain
	Signer       hotstuff.Signer
	Verifier     hotstuff.Verifier
	Executor     hotstuff.Executor
	Acceptor     hotstuff.Acceptor
	Synchronizer hotstuff.ViewSynchronizer
}

// NewBuilder returns a new Builder with default values
func NewBuilder(cfg hotstuff.Config, executor hotstuff.Executor, acceptor hotstuff.Acceptor, synchronizer hotstuff.ViewSynchronizer) *Builder {
	signer, verifier := ecdsa.New(cfg)
	return &Builder{
		Config:       cfg,
		BlockChain:   blockchain.New(100),
		Signer:       signer,
		Verifier:     verifier,
		Executor:     executor,
		Acceptor:     acceptor,
		Synchronizer: synchronizer,
	}
}

// Build returns a new chained HotStuff instance
func (b *Builder) Build() hotstuff.Consensus {
	hs := &chainedhotstuff{
		cfg:          b.Config,
		blocks:       b.BlockChain,
		signer:       b.Signer,
		verifier:     b.Verifier,
		executor:     b.Executor,
		acceptor:     b.Acceptor,
		synchronizer: b.Synchronizer,
	}
	hs.init()
	return hs
}
