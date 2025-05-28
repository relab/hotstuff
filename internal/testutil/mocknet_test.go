package testutil_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
)

func TestSenderUnicast(t *testing.T) {
	// setup
	voteCount := uint(0)
	replicaCount := uint(4)
	cfg := testutil.MockReplicaConfig{
		EventLoopBufferSize: replicaCount - 1,
	}
	network := testutil.NewMockNetwork(t, replicaCount, cfg)
	// make the leader count votes
	leaderID := hotstuff.ID(1)
	leader := network.Replica(leaderID)
	leader.EventLoop.RegisterHandler(hotstuff.PartialCert{}, func(_ any) {
		voteCount++
	})
	// tell voters to send the vote to the leader
	for _, id := range network.ReplicaIDs() {
		if leaderID == id {
			continue
		}
		r := network.Replica(leaderID)
		r.Sender.Vote(leaderID, hotstuff.PartialCert{})
	}
	// run all replica's eventloops
	for _, id := range network.ReplicaIDs() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		r := network.Replica(id)
		r.EventLoop.Run(ctx)
	}
	// verify
	want := uint(replicaCount - 1)
	if voteCount != want {
		t.Logf("want: %d, got %d", want, voteCount)
		t.Fail()
	}
}

func TestSenderBroadcast(t *testing.T) {
	// setup
	timeoutCount := uint(0)
	replicaCount := uint(4)
	cfg := testutil.MockReplicaConfig{
		EventLoopBufferSize: 1,
	}
	network := testutil.NewMockNetwork(t, replicaCount, cfg)
	// everyone will listen for a timeout message
	for _, id := range network.ReplicaIDs() {
		r := network.Replica(id)
		r.EventLoop.RegisterHandler(hotstuff.TimeoutMsg{}, func(_ any) {
			timeoutCount++
		})
	}
	// one replica will emit the timeout message
	emitterID := hotstuff.ID(1)
	emitter := network.Replica(emitterID)
	emitter.Sender.Timeout(hotstuff.TimeoutMsg{})
	// run all replica eventloops, including the one emitting the message
	for _, id := range network.ReplicaIDs() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		r := network.Replica(id)
		r.EventLoop.Run(ctx)
	}
	// verify
	want := replicaCount - 1
	if timeoutCount != want {
		t.Logf("unexpected number of responses - want: %d, got %d", want, timeoutCount)
		t.Fail()
	}
}

func TestSenderRequestBlock(t *testing.T) {
	// setup
	replicaCount := uint(4)
	cfg := testutil.MockReplicaConfig{
		EventLoopBufferSize: 1,
	}
	network := testutil.NewMockNetwork(t, replicaCount, cfg)
	storer := network.Replica(1)
	config := core.NewRuntimeConfig(storer.ID, testutil.GenerateECDSAKey(t))
	auth := cert.NewAuthority(
		config,
		storer.BlockChain,
		ecdsa.New(config),
	)
	block := testutil.CreateBlock(t, auth)
	storer.BlockChain.Store(block)
	requester := network.Replica(2)
	foundBlock, ok := requester.Sender.RequestBlock(nil, block.Hash())
	if !ok {
		t.Logf("expected block to be found")
		t.Fail()
	}
	if !bytes.Equal(block.ToBytes(), foundBlock.ToBytes()) {
		t.Logf("expected the blocks to be equal")
		t.Fail()
	}
}
