package chainedhotstuff

import (
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/leaderrotation"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/synchronizer"
)

// DefaultModules returns a default set of modules that are suitable for chainedhotstuff.
// You must provide your own implementations of the Acceptor, Config, CommandQueue, and Executor interfaces.
func DefaultModules(replicaConfig config.ReplicaConfig, duration synchronizer.ViewDuration) modules.Builder {
	builder := modules.NewBuilder(replicaConfig.ID, replicaConfig.PrivateKey)
	signer := crypto.NewCache(ecdsa.New(), 2*len(replicaConfig.Replicas))
	builder.Register(
		New(),
		synchronizer.New(duration),
		leaderrotation.NewRoundRobin(),
		blockchain.New(100),
		consensus.NewVotingMachine(),
		signer,
	)
	return builder
}
