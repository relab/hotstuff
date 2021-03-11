package gorums

import (
	"bytes"
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
	"github.com/relab/hotstuff/crypto"
	ecdsacrypto "github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/internal/testutil"
	"google.golang.org/grpc/credentials"
)

func TestConnect(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		const n = 4
		td := setup(t, n)
		ctrl := gomock.NewController(t)
		_, teardown := createServers(t, td, ctrl)
		defer teardown()
		cfg := NewConfig(td.cfg)

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
		td := setup(t, n)
		ctrl := gomock.NewController(t)
		cfg, mocks, teardown := createConfig(t, td, ctrl)
		defer teardown()

		qc := ecdsacrypto.NewQuorumCert(make(map[hotstuff.ID]*ecdsacrypto.Signature), hotstuff.GetGenesis().Hash())
		proposal := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "foo", 1, 1)
		// the configuration has ID 1, so we won't be receiving any proposal for that replica.
		c := make(chan struct{}, n-1)
		for _, mock := range mocks[1:] {
			mock.EXPECT().OnPropose(gomock.AssignableToTypeOf(proposal)).Do(func(block *hotstuff.Block) {
				if block.Hash() != proposal.Hash() {
					t.Error("hash mismatch")
				}
				c <- struct{}{}
			})
		}

		cfg.Propose(proposal)
		for i := 0; i < n-1; i++ {
			<-c
		}
	}
	runBoth(t, run)
}

func TestVote(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		const n = 4
		td := setup(t, n)
		ctrl := gomock.NewController(t)
		cfg, mocks, teardown := createConfig(t, td, ctrl)
		defer teardown()
		signer, _ := ecdsacrypto.New(cfg)

		qc := ecdsacrypto.NewQuorumCert(make(map[hotstuff.ID]*ecdsacrypto.Signature), hotstuff.GetGenesis().Hash())
		proposal := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "foo", 1, 1)
		pc := testutil.CreatePC(t, proposal, signer)

		c := make(chan struct{})
		mocks[1].EXPECT().OnVote(gomock.AssignableToTypeOf(pc)).Do(func(vote hotstuff.PartialCert) {
			if !bytes.Equal(pc.ToBytes(), vote.ToBytes()) {
				t.Error("The received partial certificate differs from the original.")
			}
			close(c)
		})

		replica, ok := cfg.Replica(2)
		if !ok {
			t.Fatalf("Failed to find replica with ID 2")
		}

		replica.Vote(pc)
		<-c
	}
	runBoth(t, run)
}

func TestTimeout(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		const n = 4
		td := setup(t, n)
		ctrl := gomock.NewController(t)
		cfg, _mocks, teardown := createConfig(t, td, ctrl)
		defer teardown()
		signer, _ := ecdsacrypto.New(cfg)

		qc := ecdsacrypto.NewQuorumCert(make(map[hotstuff.ID]*ecdsacrypto.Signature), hotstuff.GetGenesis().Hash())
		timeout := &hotstuff.TimeoutMsg{
			ID:        1,
			View:      1,
			HighQC:    qc,
			Signature: testutil.Sign(t, hotstuff.View(1).ToHash(), signer),
		}

		c := make(chan struct{}, n-1)
		for _, mock := range _mocks[1:] {

			pm := mocks.NewMockViewSynchronizer(ctrl)
			pm.EXPECT().OnRemoteTimeout(gomock.AssignableToTypeOf(timeout)).Do(func(tm *hotstuff.TimeoutMsg) {
				if !reflect.DeepEqual(timeout, tm) {
					t.Fatalf("expected timeouts to be equal. got: %v, want: %v", tm, timeout)
				}
				c <- struct{}{}
			})

			mock.EXPECT().Synchronizer().Return(pm)
		}

		cfg.Timeout(timeout)
		for i := 0; i < n-1; i++ {
			<-c
		}
	}
	runBoth(t, run)
}

type testData struct {
	n         int
	cfg       config.ReplicaConfig
	listeners []net.Listener
	keys      []*ecdsa.PrivateKey
}

type setupFunc func(t *testing.T, n int) testData

func setupReplicas(t *testing.T, n int) testData {
	t.Helper()

	listeners := make([]net.Listener, n)
	keys := make([]*ecdsa.PrivateKey, 0, n)
	replicas := make([]*config.ReplicaInfo, 0, n)

	// generate keys and replicaInfo
	for i := 0; i < n; i++ {
		listeners[i] = testutil.CreateTCPListener(t)
		keys = append(keys, testutil.GenerateKey(t))
		replicas = append(replicas, &config.ReplicaInfo{
			ID:      hotstuff.ID(i) + 1,
			Address: listeners[i].Addr().String(),
			PubKey:  &keys[i].PublicKey,
		})
	}

	cfg := config.NewConfig(1, keys[0], nil)
	for _, replica := range replicas {
		cfg.Replicas[replica.ID] = replica
	}

	return testData{n, *cfg, listeners, keys}
}

func setupTLS(t *testing.T, n int) testData {
	t.Helper()
	td := setupReplicas(t, n)

	certificates := make([]*x509.Certificate, 0, n)

	caPK := testutil.GenerateKey(t)
	ca, err := crypto.GenerateRootCert(caPK)
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	for i := 0; i < n; i++ {
		cert, err := crypto.GenerateTLSCert(
			hotstuff.ID(i)+1,
			[]string{"localhost", "127.0.0.1"},
			ca,
			td.cfg.Replicas[hotstuff.ID(i)+1].PubKey,
			caPK,
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

func createServers(t *testing.T, td testData, ctrl *gomock.Controller) (serverMocks []*mocks.MockConsensus, teardown func()) {
	t.Helper()
	servers := make([]*Server, td.n)
	serverMocks = make([]*mocks.MockConsensus, td.n)
	for i := range servers {
		cfg := td.cfg
		cfg.ID = hotstuff.ID(i + 1)
		cfg.PrivateKey = td.keys[i]
		servers[i] = NewServer(cfg)
		serverMocks[i] = mocks.NewMockConsensus(ctrl)
		servers[i].StartOnListener(serverMocks[i], td.listeners[i])
	}
	return serverMocks, func() {
		for _, srv := range servers {
			srv.Stop()
		}
	}
}

func createConfig(t *testing.T, td testData, ctrl *gomock.Controller) (cfg *Config, serverMocks []*mocks.MockConsensus, teardown func()) {
	t.Helper()
	serverMocks, serverTeardown := createServers(t, td, ctrl)
	cfg = NewConfig(td.cfg)
	err := cfg.Connect(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, mock := range serverMocks {
		mock.EXPECT().Config().AnyTimes().Return(cfg)
	}
	return cfg, serverMocks, func() {
		cfg.Close()
		serverTeardown()
	}
}
