package synchronizer_test

import (
	"context"
	"testing"

	"cuelang.org/go/pkg/time"
	"github.com/relab/hotstuff"

	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation/roundrobin"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/wiring"
)

type leaderRotation struct {
	modules.LeaderRotation
}

var _ modules.LeaderRotation = (*leaderRotation)(nil)

func TestAdvanceViewQC(t *testing.T) {
	t.Skip() // TODO(AlanRostem): finish this test's implementation
	const n = 4
	const cryptoName = ecdsa.ModuleName
	id := hotstuff.ID(1)
	pk := testutil.GenerateECDSAKey(t)
	depsCore := wiring.NewCore(id, "test", pk)
	sender := testutil.NewMockSender(id)
	depsSecurity, err := wiring.NewSecurity(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		sender,
		cryptoName,
	)
	if err != nil {
		t.Fatal(err)
	}

	ld := roundrobin.New(
		depsCore.RuntimeCfg(),
	)
	consensusRules := chainedhotstuff.New(
		depsCore.Logger(),
		depsSecurity.BlockChain(),
	)

	viewStates, err := protocol.NewViewStates(
		depsSecurity.BlockChain(),
		depsSecurity.Authority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	commandCache := clientpb.NewCommandCache()
	committer := consensus.NewCommitter(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsSecurity.BlockChain(),
		viewStates,
		consensusRules,
	)

	depsConsensus := wiring.NewConsensus(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecurity.BlockChain(),
		depsSecurity.Authority(),
		commandCache,
		committer,
		consensusRules,
		ld,
		consensus.NewHotStuff(
			depsCore.Logger(),
			depsCore.EventLoop(),
			depsCore.RuntimeCfg(),
			depsSecurity.BlockChain(),
			depsSecurity.Authority(),
			viewStates,
			ld,
			sender,
		),
	)

	_ = synchronizer.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecurity.Authority(),
		ld,
		testutil.FixedTimeout(1000*time.Nanosecond),
		depsConsensus.Proposer(),
		depsConsensus.Voter(),
		viewStates,
		sender,
	)

	signers := make([]*cert.Authority, 0) // TODO: add
	signers = append(signers, depsSecurity.Authority())
	for i := range n - 1 {
		id := hotstuff.ID(i + 2)
		pk := testutil.GenerateECDSAKey(t)
		core := wiring.NewCore(id, "test", pk)
		security, err := wiring.NewSecurity(
			core.EventLoop(),
			core.Logger(),
			core.RuntimeCfg(),
			testutil.NewMockSender(id),
			cryptoName,
		)
		if err != nil {
			t.Fatal(err)
		}
		signers = append(signers, security.Authority())
	}

	blockchain := depsSecurity.BlockChain()
	block := hotstuff.NewBlock(
		hotstuff.GetGenesis().Hash(),
		hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()),
		&clientpb.Batch{
			Commands: []*clientpb.Command{
				{
					Data: []byte("foo"),
				},
			},
		},
		1,
		2,
	)
	blockchain.Store(block)
	qc := testutil.CreateQC(t, block, signers)
	// synchronizer should tell hotstuff to propose
	proposer := depsConsensus.Proposer()
	commandCache.Add(&clientpb.Command{
		ClientID:       1,
		SequenceNumber: 1,
		Data:           []byte("bar"),
	})
	proposal, err := proposer.CreateProposal(1, viewStates.HighQC(), viewStates.SyncInfo())
	if err != nil {
		t.Fatal(err)
	}
	proposer.Propose(&proposal)

	_ = qc
	// s.AdvanceView(hotstuff.NewSyncInfo().WithQC(qc))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	depsCore.EventLoop().Run(ctx)

	if viewStates.View() != 2 {
		t.Errorf("wrong view: expected: %v, got: %v", 2, viewStates.View())
	}
}

/*func TestAdvanceViewTC(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	builders := testutil.CreateBuilders(t, ctrl, n)
	s := synchronizer.New()
	hs := mocks.NewMockConsensus(ctrl)
	builders[0].Add(s, testutil.FixedTimeout(100), hs)

	hl := builders.Build()
	signers := hl.Signers()

	tc := testutil.CreateTC(t, 1, signers)

	// synchronizer should tell hotstuff to propose
	hs.EXPECT().Propose(0, gomock.AssignableToTypeOf(hotstuff.NewSyncInfo()))

	s.AdvanceView(hotstuff.NewSyncInfo().WithTC(tc))

	if s.View() != 2 {
		t.Errorf("wrong view: expected: %v, got: %v", 2, s.View())
	}
}*/
