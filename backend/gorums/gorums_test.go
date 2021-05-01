package gorums

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/internal/testutil"
	"google.golang.org/grpc/credentials"
)

func TestConnect(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		const n = 4
		ctrl := gomock.NewController(t)
		td := setup(t, ctrl, n)
		builder := testutil.TestModules(t, ctrl, 1, td.keys[0])
		teardown := createServers(t, td, ctrl)
		defer teardown()
		cfg := NewConfig(td.cfg)

		builder.Register(cfg)
		builder.Build()

		err := cfg.Connect(time.Second)

		if err != nil {
			t.Error(err)
		}
	}
	runBoth(t, run)
}

func TestPropose(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		const n = 4
		ctrl := gomock.NewController(t)
		td := setup(t, ctrl, n)
		cfg, teardown := createConfig(t, td, ctrl)
		mocks := createMocks(t, ctrl, td, n)
		td.builders[0].Register(cfg)
		hl := td.builders.Build()

		defer teardown()

		qc := hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash())
		proposal := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "foo", 1, 1)
		// the configuration has ID 1, so we won't be receiving any proposal for that replica.
		c := make(chan struct{}, n-1)
		for _, mock := range mocks[1:] {
			mock.EXPECT().OnPropose(gomock.AssignableToTypeOf(hotstuff.ProposeMsg{})).Do(func(p hotstuff.ProposeMsg) {
				block := p.Block
				if block.Hash() != proposal.Hash() {
					t.Error("hash mismatch")
				}
				c <- struct{}{}
			})
		}

		ctx, cancel := context.WithCancel(context.Background())
		cfg.Propose(hotstuff.ProposeMsg{ID: 1, Block: proposal})
		for i := 1; i < n; i++ {
			go hl[i].EventLoop().Run(ctx)
			<-c
		}
		cancel()
	}
	runBoth(t, run)
}

// func TestVote(t *testing.T) {
// 	run := func(t *testing.T, setup setupFunc) {
// 		const n = 4
// 		ctrl := gomock.NewController(t)
// 		td := setup(t, ctrl, n)
// 		cfg, teardown := createConfig(t, td, ctrl)
// 		defer teardown()
// 		mocks := createMocks(t, ctrl, td, n)

// 		hl := td.builders.Build()
// 		signer := hl[0].Crypto()

// 		qc := hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash())
// 		proposal := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "foo", 1, 1)
// 		pc := testutil.CreatePC(t, proposal, signer)

// 		c := make(chan struct{})
// 		mocks[1].EXPECT().OnVote(gomock.AssignableToTypeOf(hotstuff.VoteMsg{})).Do(func(vote hotstuff.VoteMsg) {
// 			if !bytes.Equal(pc.ToBytes(), vote.PartialCert.ToBytes()) {
// 				t.Error("The received partial certificate differs from the original.")
// 			}
// 			close(c)
// 		})

// 		replica, ok := cfg.Replica(2)
// 		if !ok {
// 			t.Fatalf("Failed to find replica with ID 2")
// 		}

// 		ctx, cancel := context.WithCancel(context.Background())
// 		go hl[1].EventLoop().Run(ctx)

// 		replica.Vote(pc)
// 		<-c
// 		cancel()
// 	}
// 	runBoth(t, run)
// }

func TestTimeout(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		const n = 4
		ctrl := gomock.NewController(t)
		td := setup(t, ctrl, n)
		cfg, teardown := createConfig(t, td, ctrl)
		defer teardown()
		synchronizers := make([]*mocks.MockViewSynchronizer, n)
		for i := 0; i < n; i++ {
			synchronizers[i] = mocks.NewMockViewSynchronizer(ctrl)
			td.builders[i].Register(synchronizers[i])
		}
		hl := td.builders.Build()
		signer := hl[0].Crypto()

		qc := hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash())
		timeout := hotstuff.TimeoutMsg{
			ID:            1,
			View:          1,
			SyncInfo:      hotstuff.NewSyncInfo().WithQC(qc),
			ViewSignature: testutil.Sign(t, hotstuff.View(1).ToHash(), signer),
		}

		c := make(chan struct{}, n-1)
		for _, mock := range synchronizers[1:] {
			mock.EXPECT().Start().AnyTimes()
			mock.EXPECT().Stop().AnyTimes()
			mock.EXPECT().OnRemoteTimeout(gomock.AssignableToTypeOf(timeout)).Do(func(tm hotstuff.TimeoutMsg) {
				if !reflect.DeepEqual(timeout, tm) {
					t.Fatalf("expected timeouts to be equal. got: %v, want: %v", tm, timeout)
				}
				c <- struct{}{}
			})
		}

		ctx, cancel := context.WithCancel(context.Background())
		cfg.Timeout(timeout)
		for i := 1; i < n; i++ {
			go hl[i].EventLoop().Run(ctx)
			<-c
		}
		cancel()
	}
	runBoth(t, run)
}

type testData struct {
	n         int
	cfg       config.ReplicaConfig
	listeners []net.Listener
	keys      []hotstuff.PrivateKey
	builders  testutil.BuilderList
}

type setupFunc func(t *testing.T, ctrl *gomock.Controller, n int) testData

func setupReplicas(t *testing.T, ctrl *gomock.Controller, n int) testData {
	t.Helper()

	listeners := make([]net.Listener, n)
	keys := make([]hotstuff.PrivateKey, 0, n)
	replicas := make([]*config.ReplicaInfo, 0, n)

	// generate keys and replicaInfo
	for i := 0; i < n; i++ {
		listeners[i] = testutil.CreateTCPListener(t)
		keys = append(keys, testutil.GenerateECDSAKey(t))
		replicas = append(replicas, &config.ReplicaInfo{
			ID:      hotstuff.ID(i) + 1,
			Address: listeners[i].Addr().String(),
			PubKey:  keys[i].Public(),
		})
	}

	cfg := config.NewConfig(1, keys[0], nil)
	for _, replica := range replicas {
		cfg.Replicas[replica.ID] = replica
	}

	return testData{n, *cfg, listeners, keys, testutil.CreateBuilders(t, ctrl, n, keys...)}
}

func setupTLS(t *testing.T, ctrl *gomock.Controller, n int) testData {
	t.Helper()
	td := setupReplicas(t, ctrl, n)

	certificates := make([]*x509.Certificate, 0, n)

	caPK := testutil.GenerateECDSAKey(t)
	ca, err := keygen.GenerateRootCert(caPK.(*ecdsa.PrivateKey))
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	for i := 0; i < n; i++ {
		cert, err := keygen.GenerateTLSCert(
			hotstuff.ID(i)+1,
			[]string{"localhost", "127.0.0.1"},
			ca,
			td.cfg.Replicas[hotstuff.ID(i)+1].PubKey.(*ecdsa.PublicKey),
			caPK.(*ecdsa.PrivateKey),
		)
		if err != nil {
			t.Fatalf("Failed to generate certificate: %v", err)
		}
		certificates = append(certificates, cert)
	}

	cp := x509.NewCertPool()
	cp.AddCert(ca)
	creds := credentials.NewTLS(&tls.Config{
		RootCAs:      cp,
		ClientCAs:    cp,
		Certificates: []tls.Certificate{{Certificate: [][]byte{certificates[0].Raw}, PrivateKey: td.keys[0]}},
		ClientAuth:   tls.RequireAndVerifyClientCert,
	})

	td.cfg.Creds = creds
	return td
}

func runBoth(t *testing.T, run func(*testing.T, setupFunc)) {
	t.Helper()
	t.Run("NoTLS", func(t *testing.T) { run(t, setupReplicas) })
	t.Run("WithTLS", func(t *testing.T) { run(t, setupTLS) })
}

func createServers(t *testing.T, td testData, ctrl *gomock.Controller) (teardown func()) {
	t.Helper()
	servers := make([]*Server, td.n)
	for i := range servers {
		cfg := td.cfg
		cfg.ID = hotstuff.ID(i + 1)
		cfg.PrivateKey = td.keys[i]
		servers[i] = NewServer(cfg)
		servers[i].StartOnListener(td.listeners[i])
		td.builders[i].Register(servers[i])
	}
	return func() {
		for _, srv := range servers {
			srv.Stop()
		}
	}
}

func createConfig(t *testing.T, td testData, ctrl *gomock.Controller) (cfg *Config, teardown func()) {
	t.Helper()
	serverTeardown := createServers(t, td, ctrl)
	cfg = NewConfig(td.cfg)
	err := cfg.Connect(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, func() {
		cfg.Close()
		serverTeardown()
	}
}

func createMocks(t *testing.T, ctrl *gomock.Controller, td testData, n int) (m []*mocks.MockConsensus) {
	t.Helper()
	m = make([]*mocks.MockConsensus, n)
	for i := range m {
		m[i] = mocks.NewMockConsensus(ctrl)
		td.builders[i].Register(m[i])
	}
	return
}
