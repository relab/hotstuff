package hotstuff_test

import (
	"fmt"
	"net"
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
	"github.com/relab/hotstuff/internal/testutil"
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

// TestChainedHotstuff runs chained hotstuff with the gorums backend and expects each replica to execute 10 times.
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

	listeners := make([]net.Listener, n)
	keys := make([]*ecdsa.PrivateKey, n)
	for i := 0; i < n; i++ {
		listeners[i] = testutil.CreateTCPListener(t)
		keys[i] = createKey(t)
		id := hotstuff.ID(i + 1)
		baseCfg.Replicas[id] = &config.ReplicaInfo{
			ID:      id,
			Address: listeners[i].Addr().String(),
			PubKey:  &keys[i].PrivateKey.PublicKey,
		}
	}

	configs := make([]*gorums.Config, n)
	servers := make([]*gorums.Server, n)
	for i := 0; i < n; i++ {
		c := *baseCfg
		c.ID = hotstuff.ID(i + 1)
		c.PrivateKey = keys[i].PrivateKey
		configs[i] = gorums.NewConfig(c)
		servers[i] = gorums.NewServer(c)
	}

	replicas := make([]hotstuff.Consensus, n)
	synchronizers := make([]*synchronizer.Synchronizer, n)
	counters := make([]uint, n)
	c := make(chan struct{}, n)
	errChan := make(chan error, n)
	for i := 0; i < n; i++ {
		counter := &counters[i]
		executor := mocks.NewMockExecutor(ctrl)
		executor.EXPECT().Exec(gomock.Any()).AnyTimes().Do(func(arg hotstuff.Command) {
			if arg != hotstuff.Command("foo") {
				errChan <- fmt.Errorf("unknown command executed: got %s, want: %s", arg, "foo")
			}
			*counter++
			if *counter >= 100 {
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

	for i, server := range servers {
		server.StartOnListener(replicas[i], listeners[i])
		defer server.Stop()
	}

	for _, cfg := range configs {
		err := cfg.Connect(time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer cfg.Close()
	}

	for _, pm := range synchronizers {
		pm.Start()
	}

	for i := 0; i < n; i++ {
		select {
		case <-c:
			defer synchronizers[i].Stop()
		case err := <-errChan:
			t.Fatal(err)
		}
	}
}
