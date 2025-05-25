package hotstuffpb

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"

	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/bls12"
	"github.com/relab/hotstuff/internal/testutil"
	"go.uber.org/mock/gomock"
)

func TestConvertPartialCert(t *testing.T) {
	ctrl := gomock.NewController(t)

	key := testutil.GenerateECDSAKey(t)
	builder := modules.NewBuilder(1, key)
	testutil.TestModules(t, ctrl, 1, key, &builder)
	hs := builder.Build()

	var signer modules.Crypto
	hs.Get(&signer)

	want, err := signer.CreatePartialCert(hotstuff.GetGenesis())
	if err != nil {
		t.Fatal(err)
	}

	pb := PartialCertToProto(want)
	got := PartialCertFromProto(pb)

	if !bytes.Equal(want.ToBytes(), got.ToBytes()) {
		t.Error("Certificates don't match.")
	}
}

func TestConvertQuorumCert(t *testing.T) {
	ctrl := gomock.NewController(t)

	builders := testutil.CreateBuilders(t, ctrl, 4)
	hl := builders.Build()

	b1 := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()), "", 1, 1, time.Now())

	signatures := testutil.CreatePCs(t, b1, hl.Signers())

	var signer modules.Crypto
	hl[0].Get(&signer)

	want, err := signer.CreateQuorumCert(b1, signatures)
	if err != nil {
		t.Fatal(err)
	}

	pb := QuorumCertToProto(want)
	got := QuorumCertFromProto(pb)

	if !bytes.Equal(want.ToBytes(), got.ToBytes()) {
		t.Error("Certificates don't match.")
	}
}

func TestConvertBlock(t *testing.T) {
	qc := hotstuff.NewQuorumCert(nil, 0, hotstuff.Hash{})
	want := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "", 1, 1, time.Now())
	pb := BlockToProto(want)
	got := BlockFromProto(pb)

	if want.Hash() != got.Hash() {
		t.Error("Hashes don't match.")
	}
}

func TestConvertTimeoutCertBLS12(t *testing.T) {
	ctrl := gomock.NewController(t)

	builders := testutil.CreateBuilders(t, ctrl, 4, testutil.GenerateKeys(t, 4, testutil.GenerateBLS12Key)...)
	for i := range builders {
		builders[i].Add(crypto.New(bls12.New()))
	}
	hl := builders.Build()

	tc1 := testutil.CreateTC(t, 1, hl.Signers())

	pb := TimeoutCertToProto(tc1)
	tc2 := TimeoutCertFromProto(pb)

	var signer modules.Crypto
	hl[0].Get(&signer)

	if !signer.VerifyTimeoutCert(tc2) {
		t.Fatal("Failed to verify timeout cert")
	}
}

func TestTimeoutMsgFromProto_Issue129(t *testing.T) {
	sig := &QuorumSignature{Sig: &QuorumSignature_ECDSASigs{ECDSASigs: &ECDSAMultiSignature{Sigs: []*ECDSASignature{}}}}
	sync := &SyncInfo{QC: &QuorumCert{Sig: sig, Hash: []byte{1, 2, 3, 4}}}

	tests := []struct {
		name string
		msg  *TimeoutMsg
		want hotstuff.TimeoutMsg
	}{
		{name: "only-view", msg: &TimeoutMsg{View: 1}, want: hotstuff.TimeoutMsg{View: 1}},
		{name: "only-sync-info", msg: &TimeoutMsg{SyncInfo: sync}, want: hotstuff.TimeoutMsg{SyncInfo: SyncInfoFromProto(sync)}},
		{name: "only-msg-signature", msg: &TimeoutMsg{MsgSig: sig}, want: hotstuff.TimeoutMsg{MsgSignature: QuorumSignatureFromProto(sig)}},
		{name: "only-view-signature", msg: &TimeoutMsg{ViewSig: sig}, want: hotstuff.TimeoutMsg{ViewSignature: QuorumSignatureFromProto(sig)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TimeoutMsgFromProto(tt.msg)
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreUnexported(hotstuff.SyncInfo{})); diff != "" {
				t.Errorf("TimeoutMsgFromProto() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
