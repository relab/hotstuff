package gorums

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"net"
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

func generateKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	pk, err := crypto.GeneratePrivateKey()
	if err != nil {
		t.Errorf("Failed to generate private key: %v", err)
	}
	return pk
}

type testData struct {
	n         int
	cfg       *config.ReplicaConfig
	listeners []net.Listener
	keys      []*ecdsa.PrivateKey
}

func setupReplicas(t *testing.T, n int) testData {
	t.Helper()

	listeners := make([]net.Listener, n)
	keys := make([]*ecdsa.PrivateKey, 0, n)
	replicas := make([]*config.ReplicaInfo, 0, n)

	// generate keys and replicaInfo
	for i := 0; i < n; i++ {
		listeners[i] = testutil.CreateTCPListener(t)
		keys = append(keys, generateKey(t))
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

	return testData{n, cfg, listeners, keys}
}

func TestGorumsNoTLS(t *testing.T) {
	const n = 4
	td := setupReplicas(t, n)

	testGorums(t, td)
}

func TestGorumsTLS(t *testing.T) {
	const n = 4
	td := setupReplicas(t, n)

	certificates := make([]*x509.Certificate, 0, n)

	caPK := generateKey(t)
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
	testGorums(t, td)
}

func testGorums(t *testing.T, td testData) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConsensus := make([]*mocks.MockConsensus, 0, td.n)
	servers := make([]*Server, 0, td.n)

	// create mocks
	for i := 0; i < td.n; i++ {
		mockConsensus = append(mockConsensus, mocks.NewMockConsensus(ctrl))
	}

	// start servers
	for i := 0; i < td.n; i++ {
		c := *td.cfg
		c.ID = hotstuff.ID(i + 1)
		c.PrivateKey = td.keys[i]
		servers = append(servers, NewServer(c))
		servers[i].StartOnListener(mockConsensus[i], td.listeners[i])
	}

	// create the configuration
	client := NewConfig(*td.cfg)

	// test values
	qc := ecdsacrypto.NewQuorumCert(map[hotstuff.ID]*ecdsacrypto.Signature{}, hotstuff.GetGenesis().Hash())
	block := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "gorums_test", 1, 1)

	signer, _ := ecdsacrypto.New(client)
	vote, err := signer.Sign(block)
	if err != nil {
		t.Fatalf("Failed to create partial certificate: %v", err)
	}

	c := make(chan struct{}, 1)
	// configure mocks. server with id 1 should not receive any messages
	for i, mock := range mockConsensus {
		mock.EXPECT().Config().AnyTimes().Return(client)
		if i == 0 {
			continue
		}
		mock.EXPECT().OnPropose(gomock.AssignableToTypeOf(block)).Do(func(arg *hotstuff.Block) {
			if arg.Hash() != block.Hash() {
				t.Errorf("Block hash mismatch. got: %v, want: %v", arg, block)
			}
			c <- struct{}{}
		})
		mock.EXPECT().OnVote(gomock.AssignableToTypeOf(vote)).Do(func(arg hotstuff.PartialCert) {
			if !bytes.Equal(arg.ToBytes(), vote.ToBytes()) {
				t.Errorf("Vote mismatch. got: %v, want: %v", arg, vote)
			}
			c <- struct{}{}
		})
		mock.EXPECT().OnNewView(gomock.AssignableToTypeOf(hotstuff.NewView{})).Do(func(arg hotstuff.NewView) {
			if !bytes.Equal(arg.QC.ToBytes(), qc.ToBytes()) {
				t.Errorf("QC mismatch. got: %v, want: %v", arg, qc)
			}
			c <- struct{}{}
		})
	}

	err = client.Connect(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	client.Propose(block)
	for id, replica := range client.Replicas() {
		if id == client.ID() {
			continue
		}
		replica.Vote(vote)
		replica.NewView(hotstuff.NewView{ID: 1, View: 1, QC: qc})
	}

	for i := 0; i < (td.n-1)*3; i++ {
		<-c
	}

	client.Close()
	for _, server := range servers {
		server.Stop()
	}
}
