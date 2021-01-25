package gorums

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/crypto"
	ecdsacrypto "github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/internal/mocks"
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

func TestGorumsNoTLS(t *testing.T) {
	testGorums(t, false)
}

func TestGorumsTLS(t *testing.T) {
	testGorums(t, true)
}

func testGorums(t *testing.T, useTLS bool) {
	const n = 4 // number of replicas to start

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConsensus := make([]*mocks.MockConsensus, 0, n)
	keys := make([]*ecdsa.PrivateKey, 0, n)
	replicas := make([]*config.ReplicaInfo, 0, n)
	servers := make([]*Server, 0, n)
	certificates := make([]*x509.Certificate, 0, n)

	// generate keys and replicaInfo
	for i := 0; i < n; i++ {
		keys = append(keys, generateKey(t))
		replicas = append(replicas, &config.ReplicaInfo{
			ID:      hotstuff.ID(i) + 1,
			Address: fmt.Sprintf(":1337%d", i),
			PubKey:  &keys[i].PublicKey,
		})
	}

	var cp *x509.CertPool
	var creds credentials.TransportCredentials
	if useTLS {
		caPK := generateKey(t)
		ca, err := crypto.GenerateRootCert(caPK)
		if err != nil {
			t.Fatalf("Failed to generate CA: %v", err)
		}

		for i := 0; i < n; i++ {
			cert, err := crypto.GenerateTLSCert(hotstuff.ID(i)+1, []string{"localhost"}, ca, replicas[i].PubKey, caPK)
			if err != nil {
				t.Fatalf("Failed to generate certificate: %v", err)
			}
			certificates = append(certificates, cert)

		}
		cp = x509.NewCertPool()
		cp.AddCert(ca)
		creds = credentials.NewTLS(&tls.Config{
			RootCAs:      cp,
			ClientCAs:    cp,
			Certificates: []tls.Certificate{{Certificate: [][]byte{certificates[0].Raw}, PrivateKey: keys[0]}},
		})
	}

	cfg := config.NewConfig(1, keys[0], creds)
	for _, replica := range replicas {
		cfg.Replicas[replica.ID] = replica
	}

	// create mocks
	for i := 0; i < n; i++ {
		mockConsensus = append(mockConsensus, mocks.NewMockConsensus(ctrl))
	}

	// start servers
	for i := 0; i < n; i++ {
		c := *cfg
		c.ID = hotstuff.ID(i + 1)
		c.PrivateKey = keys[i]
		if useTLS {
			c.Creds = credentials.NewTLS(&tls.Config{
				RootCAs:      cp,
				ClientCAs:    cp,
				Certificates: []tls.Certificate{{Certificate: [][]byte{certificates[i].Raw}, PrivateKey: keys[i]}},
				ClientAuth:   tls.RequireAndVerifyClientCert,
			})
		}
		servers = append(servers, NewServer(c))
		servers[i].StartServer(mockConsensus[i])
	}

	// create the configuration
	client := NewConfig(*cfg)

	// test values
	qc := ecdsacrypto.NewQuorumCert(map[hotstuff.ID]*ecdsacrypto.Signature{}, hotstuff.GetGenesis().Hash())
	block := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "gorums_test", 1, 1)

	signer, _ := ecdsacrypto.New(client)
	signer.Sign(block)
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
		mock.EXPECT().OnNewView(gomock.AssignableToTypeOf(qc)).Do(func(arg hotstuff.QuorumCert) {
			if !bytes.Equal(arg.ToBytes(), qc.ToBytes()) {
				t.Errorf("QC mismatch. got: %v, want: %v", arg, qc)
			}
			c <- struct{}{}
		})
	}

	client.Connect(time.Second)
	client.Propose(block)
	for id, replica := range client.Replicas() {
		if id == client.ID() {
			continue
		}
		replica.Vote(vote)
		replica.NewView(qc)
	}

	for i := 0; i < (n-1)*3; i++ {
		<-c
	}

	client.Close()
	for _, server := range servers {
		server.Stop()
	}
}
