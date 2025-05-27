package cert_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/bls12"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/security/crypto/eddsa"
	"github.com/relab/hotstuff/wiring"
	"google.golang.org/grpc/credentials/insecure"
)

type dummyReplica struct {
	config     *core.RuntimeConfig
	connMd     map[string]string
	sender     modules.Sender
	depsSecure *wiring.Security
}

func genKey(t *testing.T, cryptoName string) hotstuff.PrivateKey {
	switch cryptoName {
	case ecdsa.ModuleName:
		return testutil.GenerateECDSAKey(t)
	case eddsa.ModuleName:
		return testutil.GenerateEDDSAKey(t)
	case bls12.ModuleName:
		return testutil.GenerateBLS12Key(t)
	}
	return nil
}

func createDependencies(t *testing.T, id int, cryptoName string, privKey hotstuff.PrivateKey, cacheSize int) *dummyReplica {
	t.Helper()
	core := wiring.NewCore(hotstuff.ID(id), "test", privKey)
	sender := network.NewGorumsSender(
		core.EventLoop(),
		core.Logger(),
		core.RuntimeCfg(),
		insecure.NewCredentials(),
	)
	opts := []cert.Option{}
	if cacheSize > 0 {
		opts = append(opts, cert.WithCache(cacheSize))
	}
	sec, err := wiring.NewSecurity(
		core.EventLoop(),
		core.Logger(),
		core.RuntimeCfg(),
		sender,
		cryptoName,
		opts...,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	// Needed for bls12 tests:
	metaData := core.RuntimeCfg().ConnectionMetadata()
	return &dummyReplica{
		config:     core.RuntimeCfg(),
		connMd:     metaData,
		depsSecure: sec,
		sender:     sender,
	}
}

func createDummyReplicas(t *testing.T, n int, cryptoName string, cacheSize int) (dummies []*dummyReplica) {
	dummies = make([]*dummyReplica, 0, n)
	replicas := make([]hotstuff.ReplicaInfo, 0, n)
	for id := range n {
		privKey := genKey(t, cryptoName)
		dummy := createDependencies(t, id+1, cryptoName, privKey, cacheSize)
		dummies = append(dummies, dummy)
		replicas = append(replicas, hotstuff.ReplicaInfo{
			ID:       hotstuff.ID(id + 1),
			PubKey:   privKey.Public(),
			Metadata: dummy.connMd,
		})
	}
	for _, dummy := range dummies {
		for _, replica := range replicas {
			dummy.config.AddReplica(&replica)
		}
	}
	return
}

func createBlock(t *testing.T, signer *cert.Authority) *hotstuff.Block {
	t.Helper()

	qc, err := signer.CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		t.Errorf("Could not create empty QC for genesis: %v", err)
	}

	b := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "foo", 42, 1)
	return b
}

var testData = []struct {
	cryptoName string
	cacheSize  int
}{
	{cryptoName: ecdsa.ModuleName},
	{cryptoName: eddsa.ModuleName},
	{cryptoName: bls12.ModuleName},
	{cryptoName: ecdsa.ModuleName, cacheSize: 10},
	{cryptoName: eddsa.ModuleName, cacheSize: 10},
	{cryptoName: bls12.ModuleName, cacheSize: 10},
}

func TestCreatePartialCert(t *testing.T) {
	for _, td := range testData {
		id := 1
		dummies := createDummyReplicas(t, 4, td.cryptoName, td.cacheSize)

		block, ok := dummies[0].depsSecure.BlockChain().Get(hotstuff.GetGenesis().Hash())
		if !ok {
			t.Errorf("no block")
		}

		partialCert, err := dummies[0].depsSecure.Authority().CreatePartialCert(block)
		if err != nil {
			t.Fatalf("Failed to create partial certificate: %v", err)
		}

		if partialCert.BlockHash() != block.Hash() {
			t.Error("Partial certificate hash does not match block hash!")
		}

		if signerID := partialCert.Signer(); signerID != hotstuff.ID(id) {
			t.Errorf("Wrong ID for signer in partial certificate: got: %d, want: %d", signerID, hotstuff.ID(id))
		}
	}
}

func TestVerifyPartialCert(t *testing.T) {
	for _, td := range testData {
		dummies := createDummyReplicas(t, 2, td.cryptoName, td.cacheSize)
		dummy := dummies[0]
		block := createBlock(t, dummy.depsSecure.Authority())
		dummy.depsSecure.BlockChain().Store(block)

		partialCert := testutil.CreatePC(t, block, dummy.depsSecure.Authority())

		if !dummy.depsSecure.Authority().VerifyPartialCert(partialCert) {
			t.Error("Partial Certificate was not verified.")
		}
	}
}

func TestCreateQuorumCert(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummyReplicas(t, n, td.cryptoName, td.cacheSize)
		signers := make([]*cert.Authority, 0)
		for _, dummy := range dummies {
			signers = append(signers, dummy.depsSecure.Authority())
		}
		dummy := dummies[0]
		block := createBlock(t, dummy.depsSecure.Authority())
		pcs := testutil.CreatePCs(t, block, signers)

		qc, err := signers[0].CreateQuorumCert(block, pcs)
		if err != nil {
			t.Fatalf("Failed to create QC: %v", err)
		}

		if qc.BlockHash() != block.Hash() {
			t.Error("Quorum certificate hash does not match block hash!")
		}
	}
}

func TestCreateTimeoutCert(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummyReplicas(t, n, td.cryptoName, td.cacheSize)
		signers := make([]modules.CryptoBase, 0)
		for _, dummy := range dummies {
			signers = append(signers, dummy.depsSecure.CryptoImpl())
		}

		timeouts := testutil.CreateTimeouts(t, 1, signers)

		tc, err := dummies[0].depsSecure.Authority().CreateTimeoutCert(1, timeouts)
		if err != nil {
			t.Fatalf("Failed to create QC: %v", err)
		}

		if tc.View() != hotstuff.View(1) {
			t.Error("Timeout certificate view does not match original view.")
		}
	}
}

func TestCreateQCWithOneSig(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummyReplicas(t, n, td.cryptoName, td.cacheSize)
		signers := make([]*cert.Authority, 0)
		for _, dummy := range dummies {
			signers = append(signers, dummy.depsSecure.Authority())
		}
		dummy := dummies[0]
		block := createBlock(t, dummy.depsSecure.Authority())
		pcs := testutil.CreatePCs(t, block, signers)
		_, err := signers[0].CreateQuorumCert(block, pcs[:1])
		if err == nil {
			t.Fatal("Expected error when creating QC with only one signature")
		}
	}
}

func TestCreateQCWithOverlappingSigs(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummyReplicas(t, n, td.cryptoName, td.cacheSize)
		signers := make([]*cert.Authority, 0)
		for _, dummy := range dummies {
			signers = append(signers, dummy.depsSecure.Authority())
		}
		dummy := dummies[0]
		block := createBlock(t, dummy.depsSecure.Authority())
		pcs := testutil.CreatePCs(t, block, signers)
		pcs = append(pcs, pcs[0])
		_, err := signers[0].CreateQuorumCert(block, pcs)
		if err == nil {
			t.Fatal("Expected error when creating QC with overlapping signatures")
		}
	}
}

func TestVerifyGenesisQC(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummyReplicas(t, n, td.cryptoName, td.cacheSize)
		signers := make([]*cert.Authority, 0)
		for _, dummy := range dummies {
			signers = append(signers, dummy.depsSecure.Authority())
		}

		genesisQC, err := signers[0].CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
		if err != nil {
			t.Fatal(err)
		}
		if !signers[1].VerifyQuorumCert(dummies[0].config.QuorumSize(), genesisQC) {
			t.Error("Genesis QC was not verified!")
		}
	}
}

func TestVerifyQuorumCert(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummyReplicas(t, n, td.cryptoName, td.cacheSize)
		signers := make([]*cert.Authority, 0)
		signedBlock := createBlock(t, dummies[0].depsSecure.Authority())
		for _, dummy := range dummies {
			signers = append(signers, dummy.depsSecure.Authority())
			dummy.depsSecure.BlockChain().Store(signedBlock)
		}

		qc := testutil.CreateQC(t, signedBlock, signers)

		for i, verifier := range signers {
			qSize := dummies[i].config.QuorumSize()
			if !verifier.VerifyQuorumCert(qSize, qc) {
				t.Errorf("verifier %d failed to verify QC! (qsize=%d)", i+1, qSize)
			}
		}
	}
}

func TestVerifyTimeoutCert(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummyReplicas(t, n, td.cryptoName, td.cacheSize)
		signers0 := make([]*cert.Authority, 0)
		signers1 := make([]modules.CryptoBase, 0)
		signedBlock := createBlock(t, dummies[0].depsSecure.Authority())
		for _, dummy := range dummies {
			signers0 = append(signers0, dummy.depsSecure.Authority())
			signers1 = append(signers1, dummy.depsSecure.CryptoImpl())
			dummy.depsSecure.BlockChain().Store(signedBlock)
		}

		tc := testutil.CreateTC(t, 1, signers0, signers1)

		for i, verifier := range signers0 {
			if !verifier.VerifyTimeoutCert(dummies[0].config.QuorumSize(), tc) {
				t.Errorf("verifier %d failed to verify TC!", i+1)
			}
		}
	}
}

func TestVerifyAggregateQC(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummyReplicas(t, n, td.cryptoName, td.cacheSize)
		signers0 := make([]*cert.Authority, 0)
		signers1 := make([]modules.CryptoBase, 0)
		signedBlock := createBlock(t, dummies[0].depsSecure.Authority())
		for _, dummy := range dummies {
			signers0 = append(signers0, dummy.depsSecure.Authority())
			signers1 = append(signers1, dummy.depsSecure.CryptoImpl())
			dummy.depsSecure.BlockChain().Store(signedBlock)
		}

		timeouts := testutil.CreateTimeouts(t, 1, signers1)
		aggQC, err := signers0[0].CreateAggregateQC(1, timeouts)
		if err != nil {
			t.Fatal(err)
		}

		highQC, ok := signers0[0].VerifyAggregateQC(dummies[0].config.QuorumSize(), aggQC)
		if !ok {
			t.Fatal("AggregateQC was not verified")
		}

		if highQC.BlockHash() != hotstuff.GetGenesis().Hash() {
			t.Fatal("Wrong hash for highQC")
		}
	}
}
