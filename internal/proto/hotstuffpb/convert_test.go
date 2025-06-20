package hotstuffpb_test

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/security/cert"

	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/security/crypto/bls12"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
)

func TestConvertPartialCert(t *testing.T) {
	key := testutil.GenerateECDSAKey(t)
	cfg := core.NewRuntimeConfig(1, key)
	crypt := ecdsa.New(cfg)
	signer := cert.NewAuthority(cfg, nil, crypt)

	want, err := signer.CreatePartialCert(hotstuff.GetGenesis())
	if err != nil {
		t.Fatal(err)
	}

	pb := hotstuffpb.PartialCertToProto(want)
	got := hotstuffpb.PartialCertFromProto(pb)

	if !bytes.Equal(want.ToBytes(), got.ToBytes()) {
		t.Error("Certificates don't match.")
	}
}

func TestConvertQuorumCert(t *testing.T) {
	n := 4
	signers := make([]*cert.Authority, n)
	for i := range n {
		key := testutil.GenerateECDSAKey(t)
		cfg := core.NewRuntimeConfig(hotstuff.ID(i+1), key)
		crypt := ecdsa.New(cfg)
		signer := cert.NewAuthority(cfg, nil, crypt)
		signers[i] = signer
	}

	b1 := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()), &clientpb.Batch{}, 1, 1)

	signatures := testutil.CreatePCs(t, b1, signers)

	want, err := signers[0].CreateQuorumCert(b1, signatures)
	if err != nil {
		t.Fatal(err)
	}

	pb := hotstuffpb.QuorumCertToProto(want)
	got := hotstuffpb.QuorumCertFromProto(pb)

	if !bytes.Equal(want.ToBytes(), got.ToBytes()) {
		t.Error("Certificates don't match.")
	}
}

func TestConvertBlock(t *testing.T) {
	qc := hotstuff.NewQuorumCert(nil, 0, hotstuff.Hash{})
	want := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, &clientpb.Batch{}, 1, 1)
	pb := hotstuffpb.BlockToProto(want)
	got := hotstuffpb.BlockFromProto(pb)

	if want.Hash() != got.Hash() {
		t.Error("Hashes don't match.")
	}
}

func TestConvertTimeoutCertBLS12(t *testing.T) {
	n := 4
	cfgs := make(map[hotstuff.ID]*core.RuntimeConfig)
	replicaInfos := make(map[hotstuff.ID]*hotstuff.ReplicaInfo)
	for i := range n {
		id := hotstuff.ID(i + 1)
		key := testutil.GenerateBLS12Key(t)
		cfgs[id] = core.NewRuntimeConfig(id, key)
		pub := key.Public()
		replicaInfos[id] = &hotstuff.ReplicaInfo{ID: id, PubKey: pub}
	}
	// add info about each replica to each other
	for i := range n {
		id := hotstuff.ID(i + 1)
		for id2 := range replicaInfos {
			cfgs[id].AddReplica(replicaInfos[id2])
		}
	}

	signers := make([]*cert.Authority, n)
	for i := range n {
		id := hotstuff.ID(i + 1)
		crypt, err := bls12.New(cfgs[id])
		if err != nil {
			t.Fatal(err)
		}
		signer := cert.NewAuthority(cfgs[id], nil, crypt)
		signers[i] = signer
		meta := cfgs[id].ConnectionMetadata()
		err = cfgs[id].SetReplicaMetadata(id, meta)
		if err != nil {
			t.Fatal(err)
		}
	}

	tc1 := testutil.CreateTCOld(t, 1, signers)

	pb := hotstuffpb.TimeoutCertToProto(tc1)
	tc2 := hotstuffpb.TimeoutCertFromProto(pb)

	signer := signers[0]

	if err := signer.VerifyTimeoutCert(cfgs[1].QuorumSize(), tc2); err != nil {
		t.Fatalf("Failed to verify timeout cert: %v", err)
	}
}

func TestTimeoutMsgFromProto_Issue129(t *testing.T) {
	sig := &hotstuffpb.QuorumSignature{Sig: &hotstuffpb.QuorumSignature_ECDSASigs{ECDSASigs: &hotstuffpb.ECDSAMultiSignature{Sigs: []*hotstuffpb.ECDSASignature{}}}}
	sync := &hotstuffpb.SyncInfo{QC: &hotstuffpb.QuorumCert{Sig: sig, Hash: []byte{1, 2, 3, 4}}}

	tests := []struct {
		name string
		msg  *hotstuffpb.TimeoutMsg
		want hotstuff.TimeoutMsg
	}{
		{name: "only-view", msg: &hotstuffpb.TimeoutMsg{View: 1}, want: hotstuff.TimeoutMsg{View: 1}},
		{name: "only-sync-info", msg: &hotstuffpb.TimeoutMsg{SyncInfo: sync}, want: hotstuff.TimeoutMsg{SyncInfo: hotstuffpb.SyncInfoFromProto(sync)}},
		{name: "only-msg-signature", msg: &hotstuffpb.TimeoutMsg{MsgSig: sig}, want: hotstuff.TimeoutMsg{MsgSignature: hotstuffpb.QuorumSignatureFromProto(sig)}},
		{name: "only-view-signature", msg: &hotstuffpb.TimeoutMsg{ViewSig: sig}, want: hotstuff.TimeoutMsg{ViewSignature: hotstuffpb.QuorumSignatureFromProto(sig)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hotstuffpb.TimeoutMsgFromProto(tt.msg)
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreUnexported(hotstuff.SyncInfo{})); diff != "" {
				t.Errorf("TimeoutMsgFromProto() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
