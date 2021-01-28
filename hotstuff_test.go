package hotstuff_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend/gorums"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/consensus/chainedhotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/leaderrotation"
	"github.com/relab/hotstuff/synchronizer"
)

func createKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := crypto.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	return &ecdsa.PrivateKey{PrivateKey: key}
}

func createGorums(t *testing.T, baseCfg config.ReplicaConfig) (cfg *gorums.Config, start func(hotstuff.Consensus), teardown func()) {
	t.Helper()
	cfg = gorums.NewConfig(baseCfg)
	srv := gorums.NewServer(baseCfg)
	start = func(c hotstuff.Consensus) {
		t.Helper()
		if err := srv.StartServer(c); err != nil {
			t.Fatalf("Failed to start gorums server: %v", err)
		}
	}
	teardown = func() {
		t.Helper()
		cfg.Close()
		srv.Stop()
	}
	return
}

func TestChainedHotstuff(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	acceptor := mocks.NewMockAcceptor(ctrl)
	acceptor.EXPECT().Accept(gomock.Any()).AnyTimes().Return(true)
	commands := mocks.NewMockCommandQueue(ctrl)
	commands.EXPECT().GetCommand().AnyTimes().DoAndReturn(func() *hotstuff.Command {
		cmd := hotstuff.Command("foo")
		return &cmd
	})

	baseCfg := config.NewConfig(0, nil, nil)

	keys := make([]*ecdsa.PrivateKey, n)
	for i := 0; i < n; i++ {
		keys[i] = createKey(t)
		id := hotstuff.ID(i + 1)
		baseCfg.Replicas[id] = &config.ReplicaInfo{
			ID:      id,
			Address: fmt.Sprintf(":1337%d", id),
			PubKey:  &keys[i].PrivateKey.PublicKey,
		}
	}

	configs := make([]*gorums.Config, n)
	starters := make([]func(hotstuff.Consensus), n)
	for i := 0; i < n; i++ {
		c := *baseCfg
		c.ID = hotstuff.ID(i + 1)
		c.PrivateKey = keys[i].PrivateKey
		cfg, start, teardown := createGorums(t, c)
		configs[i] = cfg
		starters[i] = start
		defer teardown()
	}

	replicas := make([]hotstuff.Consensus, n)
	synchronizers := make([]*synchronizer.Synchronizer, n)
	counters := make([]uint, n)
	c := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		counter := &counters[i]
		pm := &synchronizers[i]
		executor := mocks.NewMockExecutor(ctrl)
		executor.EXPECT().Exec(gomock.Any()).AnyTimes().Do(func(arg hotstuff.Command) {
			if arg != hotstuff.Command("foo") {
				t.Fatalf("Unknown command executed: got %s, want: %s", arg, "foo")
			}
			*counter++
			if *counter >= 10 {
				(*pm).Stop()
				c <- struct{}{}
			}
		})
		synchronizers[i] = synchronizer.New(leaderrotation.NewRoundRobin(configs[i]), 500*time.Millisecond)
		replicas[i] = chainedhotstuff.Builder{
			Config:       configs[i],
			Synchronizer: synchronizers[i],
			Acceptor:     acceptor,
			CommandQueue: commands,
			Executor:     executor,
		}.Build()
	}

	for i, start := range starters {
		start(replicas[i])
	}

	for _, cfg := range configs {
		cfg.Connect(time.Second)
	}

	for _, pm := range synchronizers {
		pm.Start()
	}

	for i := 0; i < n; i++ {
		<-c
	}
}
