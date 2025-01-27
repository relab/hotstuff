package hotstuffpb_test

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/certauth"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/netconfig"

	"github.com/relab/hotstuff/crypto/bls12"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/testutil"
	"go.uber.org/mock/gomock"
)

func TestConvertPartialCert(t *testing.T) {
	ctrl := gomock.NewController(t)

	key := testutil.GenerateECDSAKey(t)
	builder := core.NewBuilder(1, key)
	testutil.TestModules(t, ctrl, 1, key, &builder)
	hs := builder.Build()

	var signer core.CertAuth
	hs.Get(&signer)

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
	ctrl := gomock.NewController(t)

	builders := testutil.CreateBuilders(t, ctrl, 4)
	hl := builders.Build()

	b1 := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()), "", 1, 1)

	signatures := testutil.CreatePCs(t, b1, hl.Signers())

	var signer core.CertAuth
	hl[0].Get(&signer)

	want, err := signer.CreateQuorumCert(b1, signatures)
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
	want := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "", 1, 1)
	pb := hotstuffpb.BlockToProto(want)
	got := hotstuffpb.BlockFromProto(pb)

	if want.Hash() != got.Hash() {
		t.Error("Hashes don't match.")
	}
}

func TestConvertTimeoutCertBLS12(t *testing.T) {
	ctrl := gomock.NewController(t)

	builders := testutil.CreateBuilders(t, ctrl, 4, testutil.GenerateKeys(t, 4, testutil.GenerateBLS12Key)...)
	for i := range builders {
		opt := builders[i].Options()
		var cfg *netconfig.Config
		var blockChain *blockchain.BlockChain
		logger := logging.New("test")
		builders[i].Add(certauth.New(bls12.New(
			cfg,
			logger,
			opt,
		),
			blockChain,
			cfg,
			logger,
		))
	}
	hl := builders.Build()

	tc1 := testutil.CreateTC(t, 1, hl.Signers())

	pb := hotstuffpb.TimeoutCertToProto(tc1)
	tc2 := hotstuffpb.TimeoutCertFromProto(pb)

	var signer core.CertAuth
	hl[0].Get(&signer)

	if !signer.VerifyTimeoutCert(tc2) {
		t.Fatal("Failed to verify timeout cert")
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
