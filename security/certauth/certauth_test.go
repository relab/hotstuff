package certauth_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/security/certauth"
	"github.com/relab/hotstuff/security/crypto/bls12"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/security/crypto/eddsa"
)

type dummyReplica struct {
	netComps *orchestration.NetworkComponents
	secComps *orchestration.SecurityComponents
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

func createComponents(t *testing.T, id int, cryptoName string, privKey hotstuff.PrivateKey, cacheSize int) *dummyReplica {
	t.Helper()
	core := orchestration.NewCoreComponents(hotstuff.ID(id), "test", privKey)
	net := orchestration.NewNetworkComponents(core, nil)
	sec, err := orchestration.NewSecurityComponents(core, net, cryptoName, cacheSize)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return &dummyReplica{
		secComps: sec,
		netComps: net,
	}
}

func createDummyReplicas(t *testing.T, n int, cryptoName string, cacheSize int) (dummies []*dummyReplica) {
	dummies = make([]*dummyReplica, 0, n)
	replicas := make([]hotstuff.ReplicaInfo, 0, n)
	for id := range n {
		privKey := genKey(t, cryptoName)
		dummy := createComponents(t, id+1, cryptoName, privKey, cacheSize)
		dummies = append(dummies, dummy)
		replicas = append(replicas, hotstuff.ReplicaInfo{
			ID:     hotstuff.ID(id + 1),
			PubKey: privKey.Public(),
		})
	}

	for _, dummy := range dummies {
		for _, replica := range replicas {
			dummy.netComps.Config.AddReplica(&replica)
		}
	}

	return
}

func TestCreatePartialCert(t *testing.T) {
	testData := []struct {
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
	for _, td := range testData {
		id := 1
		dummies := createDummyReplicas(t, 2, td.cryptoName, td.cacheSize)

		block, ok := dummies[0].secComps.BlockChain.Get(hotstuff.GetGenesis().Hash())
		if !ok {
			t.Errorf("no block")
		}

		partialCert, err := dummies[0].secComps.CertAuth.CreatePartialCert(block)
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
	block := hotstuff.NewBlock(
		hotstuff.GetGenesis().Hash(),
		hotstuff.GetGenesis().QuorumCert(),
		"hello",
		1, 1,
	)
	testData := []struct {
		cryptoName string
		cacheSize  int
		block      *hotstuff.Block
	}{
		{cryptoName: ecdsa.ModuleName, block: block},
		{cryptoName: eddsa.ModuleName, block: block},
		{cryptoName: bls12.ModuleName, block: block},
		{cryptoName: ecdsa.ModuleName, cacheSize: 10, block: block},
		{cryptoName: eddsa.ModuleName, cacheSize: 10, block: block},
		{cryptoName: bls12.ModuleName, cacheSize: 10, block: block},
	}

	for _, td := range testData {
		dummies := createDummyReplicas(t, 2, td.cryptoName, td.cacheSize)
		dummy := dummies[0]
		dummy.secComps.BlockChain.Store(td.block)

		partialCert := testutil.CreatePC(t, td.block, dummy.secComps.CertAuth)

		if !dummy.secComps.CertAuth.VerifyPartialCert(partialCert) {
			t.Error("Partial Certificate was not verified.")
		}
	}
}

func TestCreateQuorumCert(t *testing.T) {

	block := hotstuff.NewBlock(
		hotstuff.GetGenesis().Hash(),
		hotstuff.GetGenesis().QuorumCert(),
		"hello",
		1, 1,
	)
	testData := []struct {
		cryptoName string
		cacheSize  int
		block      *hotstuff.Block
	}{
		{cryptoName: ecdsa.ModuleName, block: block},
		{cryptoName: eddsa.ModuleName, block: block},
		{cryptoName: bls12.ModuleName, block: block},
		{cryptoName: ecdsa.ModuleName, cacheSize: 10, block: block},
		{cryptoName: eddsa.ModuleName, cacheSize: 10, block: block},
		{cryptoName: bls12.ModuleName, cacheSize: 10, block: block},
	}

	for _, td := range testData {
		const n = 2
		dummies := createDummyReplicas(t, n, td.cryptoName, td.cacheSize)
		signers := make([]*certauth.CertAuthority, 0)
		for _, dummy := range dummies {
			signers = append(signers, dummy.secComps.CertAuth)
		}

		pcs := testutil.CreatePCs(t, td.block, signers)

		qc, err := signers[0].CreateQuorumCert(td.block, pcs)
		if err != nil {
			t.Fatalf("Failed to create QC: %v", err)
		}

		if qc.BlockHash() != td.block.Hash() {
			t.Error("Quorum certificate hash does not match block hash!")
		}
	}
}

/*

func TestCreateTimeoutCert(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)

		td := setup(t, ctrl, 4)

		timeouts := testutil.CreateTimeouts(t, 1, td.signers)

		tc, err := td.signers[0].CreateTimeoutCert(1, timeouts)
		if err != nil {
			t.Fatalf("Failed to create QC: %v", err)
		}

		if tc.View() != hotstuff.View(1) {
			t.Error("Timeout certificate view does not match original view.")
		}
	}
	runAll(t, run)
}


func TestCreateQCWithOneSig(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)
		td := setup(t, ctrl, 4)
		pcs := testutil.CreatePCs(t, td.block, td.signers)
		_, err := td.signers[0].CreateQuorumCert(td.block, pcs[:1])
		if err == nil {
			t.Fatal("Expected error when creating QC with only one signature")
		}
	}
	runAll(t, run)
}

func TestCreateQCWithOverlappingSigs(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)
		td := setup(t, ctrl, 4)
		pcs := testutil.CreatePCs(t, td.block, td.signers)
		pcs = append(pcs, pcs[0])
		_, err := td.signers[0].CreateQuorumCert(td.block, pcs)
		if err == nil {
			t.Fatal("Expected error when creating QC with overlapping signatures")
		}
	}
	runAll(t, run)
}

func TestVerifyGenesisQC(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)

		td := setup(t, ctrl, 4)

		genesisQC, err := td.signers[0].CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
		if err != nil {
			t.Fatal(err)
		}
		if !td.verifiers[0].VerifyQuorumCert(genesisQC) {
			t.Error("Genesis QC was not verified!")
		}
	}
	runAll(t, run)
}

func TestVerifyQuorumCert(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)

		td := setup(t, ctrl, 4)

		qc := testutil.CreateQC(t, td.block, td.signers)

		for i, verifier := range td.verifiers {
			if !verifier.VerifyQuorumCert(qc) {
				t.Errorf("verifier %d failed to verify QC!", i+1)
			}
		}
	}
	runAll(t, run)
}

func TestVerifyTimeoutCert(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)

		td := setup(t, ctrl, 4)

		tc := testutil.CreateTC(t, 1, td.signers)

		for i, verifier := range td.verifiers {
			if !verifier.VerifyTimeoutCert(tc) {
				t.Errorf("verifier %d failed to verify TC!", i+1)
			}
		}
	}
	runAll(t, run)
}

func TestVerifyAggregateQC(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)
		td := setup(t, ctrl, 4)

		timeouts := testutil.CreateTimeouts(t, 1, td.signers)
		aggQC, err := td.signers[0].CreateAggregateQC(1, timeouts)
		if err != nil {
			t.Fatal(err)
		}

		highQC, ok := td.signers[0].VerifyAggregateQC(aggQC)
		if !ok {
			t.Fatal("AggregateQC was not verified")
		}

		if highQC.BlockHash() != hotstuff.GetGenesis().Hash() {
			t.Fatal("Wrong hash for highQC")
		}
	}
	runAll(t, run)
}


////////////////////////////////// UTILS BEYOND HERE //////////////////////////////////
func runAll(t *testing.T, run func(*testing.T, setupFunc)) {
	t.Helper()
	t.Run("Ecdsa", func(t *testing.T) { run(t, setup(NewBase(ecdsa.New), testutil.GenerateECDSAKey)) })
	t.Run("Cache+Ecdsa", func(t *testing.T) { run(t, setup(NewCache(ecdsa.New), testutil.GenerateECDSAKey)) })
	t.Run("Eddsa", func(t *testing.T) { run(t, setup(NewBase(eddsa.New), testutil.GenerateEDDSAKey)) })
	t.Run("Cache+Eddsa", func(t *testing.T) { run(t, setup(NewCache(eddsa.New), testutil.GenerateEDDSAKey)) })
	t.Run("BLS12-381", func(t *testing.T) { run(t, setup(NewBase(bls12.New), testutil.GenerateBLS12Key)) })
	t.Run("Cache+BLS12-381", func(t *testing.T) { run(t, setup(NewCache(bls12.New), testutil.GenerateBLS12Key)) })
}

func createBlock(t *testing.T, signer core.CertAuth) *hotstuff.Block {
	t.Helper()

	qc, err := signer.CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		t.Errorf("Could not create empty QC for genesis: %v", err)
	}

	b := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "foo", 42, 1)
	return b
}

type keyFunc func(t *testing.T) hotstuff.PrivateKey
type setupFunc func(*testing.T, *gomock.Controller, int) testData

func setup(newFunc func() core.CertAuth, keyFunc keyFunc) setupFunc {
	return func(t *testing.T, ctrl *gomock.Controller, n int) testData {
		return newTestData(t, ctrl, n, newFunc, keyFunc)
	}
}

func NewCache(impl func() modules.CryptoBase) func() core.CertAuth {
	return func() core.CertAuth {
		return certauth.NewCache(impl(), 10)
	}
}

func NewBase(impl func() modules.CryptoBase) func() core.CertAuth {
	return func() core.CertAuth {
		return certauth.New(impl())
	}
}

type testData struct {
	signers   []core.CertAuth
	verifiers []core.CertAuth
	block     *hotstuff.Block
}

func newTestData(t *testing.T, ctrl *gomock.Controller, n int, newFunc func() core.CertAuth, keyFunc keyFunc) testData {
	t.Helper()

	bl := testutil.CreateBuilders(t, ctrl, n, testutil.GenerateKeys(t, n, keyFunc)...)
	for _, builder := range bl {
		signer := newFunc()
		builder.Add(signer)
	}
	hl := bl.Build()

	var signer core.CertAuth
	hl[0].Get(&signer)

	block := createBlock(t, signer)

	for _, mods := range hl {
		var blockChain *blockchain.BlockChain
		mods.Get(&blockChain)

		blockChain.Store(block)
	}

	return testData{
		signers:   hl.Signers(),
		verifiers: hl.Verifiers(),
		block:     block,
	}
}
*/
