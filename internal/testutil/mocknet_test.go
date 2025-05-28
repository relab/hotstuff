package testutil_test

import (
	"context"
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/testutil"
)

func TestUnicast(t *testing.T) {
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
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Millisecond)
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

func TestBroadcast(t *testing.T) {
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
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Millisecond)
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

// func TestRequestBlock(t *testing.T) {
//
// }
