package consensus_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/dependencies"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"google.golang.org/grpc/credentials/insecure"
)

// TestVote checks that a leader can collect votes on a proposal to form a QC
func TestVote(t *testing.T) {
	t.Skip() // TODO: Finish setting up this test so it passes.
	const n = 4

	type replica struct {
		eventLoop    *eventloop.EventLoop
		blockChain   *blockchain.BlockChain
		certAuth     *certauth.CertAuthority
		proposer     *consensus.Proposer
		synchronizer *synchronizer.Synchronizer
	}

	replicas := []replica{}

	for i := range n {
		id := hotstuff.ID(i + 1)
		cryptoName := ecdsa.ModuleName
		// consensusName := chainedhotstuff.ModuleName
		// leaderRotationName := leaderrotation.RoundRobinModuleName
		cacheSize := 100

		depsCore := dependencies.NewCore(id, fmt.Sprintf("hs%d", id), testutil.GenerateECDSAKey(t))
		sender := network.NewGorumsSender(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			insecure.NewCredentials(),
		)
		depsSecure, err := dependencies.NewSecurity(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			sender,
			cryptoName,
			certauth.WithCache(cacheSize),
		)
		if err != nil {
			t.Fatalf("%v", err)
		}
		_ = depsSecure
		// TODO(AlanRostem): fix
		/*depsService := dependencies.NewService(
			depsCore.Logger(),
			depsCore.EventLoop(),
			depsSecure.BlockChain(),
			nil,
		)
		depsProtocol, err := dependencies.NewProtocol(
			depsCore, depsNet, depsSecure, depsService,
			consensusName, leaderRotationName, "",
			viewduration.NewParams(0, 1*time.Millisecond, 0, 0), // TODO(AlanRostem): ensure test values are correct
		)
		if err != nil {
			t.Fatalf("%v", err)
		}
		replicas = append(replicas, replica{
			eventLoop:    depsCore.EventLoop(),
			blockChain:   depsSecure.BlockChain(),
			consensus:    depsProtocol.Consensus(),
			certAuth:     depsSecure.CertAuth(),
			synchronizer: depsProtocol.Synchronizer(),
		})*/
	}

	r := replicas[0]

	ok := false
	ctx, cancel := context.WithCancel(context.Background())
	r.eventLoop.RegisterHandler(hotstuff.NewViewMsg{}, func(_ any) {
		ok = true
		cancel()
	}, eventloop.Prioritize())

	b := testutil.NewProposeMsg(
		hotstuff.GetGenesis().Hash(),
		hotstuff.NewQuorumCert(nil, 1, hotstuff.GetGenesis().Hash()),
		"test", 1, 1,
	)

	qc := b.Block.QuorumCert()
	r.blockChain.Store(b.Block)
	// TODO: Test isn't succeeding since it hangs where consensus tries to get a command from cmdCache.
	r.proposer.Propose(1, qc, hotstuff.NewSyncInfo().WithQC(qc))

	for i, signer := range replicas {
		pc, err := signer.certAuth.CreatePartialCert(b.Block)
		if err != nil {
			t.Fatalf("Failed to create partial certificate: %v", err)
		}
		r.eventLoop.AddEvent(hotstuff.VoteMsg{ID: hotstuff.ID(i + 1), PartialCert: pc})
	}

	r.synchronizer.Start(ctx)
	r.eventLoop.Run(ctx)

	if !ok {
		t.Error("No new view event happened")
	}
}
