package consensus_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/internal/dependencies"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
)

// TestVote checks that a leader can collect votes on a proposal to form a QC
func TestVote(t *testing.T) {
	t.Skip() // TODO: Finish setting up this test so it passes.
	const n = 4

	type replica struct {
		eventLoop    *eventloop.EventLoop
		blockChain   *blockchain.BlockChain
		consensus    *consensus.Consensus
		certAuth     *certauth.CertAuthority
		synchronizer *synchronizer.Synchronizer
	}

	replicas := []replica{}

	for i := range n {
		id := hotstuff.ID(i + 1)
		cryptoName := ecdsa.ModuleName
		consensusName := chainedhotstuff.ModuleName
		leaderRotationName := leaderrotation.RoundRobinModuleName
		cacheSize := 100
		batchSize := 1

		depsCore := dependencies.NewCore(id, fmt.Sprintf("hs%d", id), testutil.GenerateECDSAKey(t))
		depsNet := dependencies.NewNetwork(depsCore, nil)
		depsSecure, err := dependencies.NewSecurity(depsCore, depsNet, cryptoName, cacheSize)
		if err != nil {
			t.Fatalf("%v", err)
		}
		depsService := dependencies.NewService(depsCore, depsSecure, batchSize, []gorums.ServerOption{})
		depsProtocol, err := dependencies.NewProtocol(
			depsCore, depsNet, depsSecure, depsService,
			false, consensusName, leaderRotationName, "",
			viewduration.NewOptions(0, 1*time.Millisecond, 0, 0), // TODO(AlanRostem): ensure test values are correct
		)
		if err != nil {
			t.Fatalf("%v", err)
		}
		replicas = append(replicas, replica{
			eventLoop:    depsCore.EventLoop,
			blockChain:   depsSecure.BlockChain,
			consensus:    depsProtocol.Consensus,
			certAuth:     depsSecure.CertAuth,
			synchronizer: depsProtocol.Synchronizer,
		})
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
	r.consensus.Propose(1, qc, hotstuff.NewSyncInfo().WithQC(qc))

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
