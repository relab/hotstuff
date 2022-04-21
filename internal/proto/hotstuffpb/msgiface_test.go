package hotstuffpb_test

import (
	"github.com/relab/hotstuff/hs"
	"testing"

	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
)

func proposeMsgStruct() *hotstuffpb.Proposal {
	block := &hotstuffpb.Block{
		Parent:   []byte("parent"),
		QC:       &hotstuffpb.QuorumCert{},
		View:     1,
		Command:  []byte("command"),
		Proposer: 1,
	}
	aggregateQC := &hotstuffpb.AggQC{
		QCs:  map[uint32]*hotstuffpb.QuorumCert{},
		Sig:  &hotstuffpb.ThresholdSignature{},
		View: 1,
	}
	return &hotstuffpb.Proposal{
		Block: block,
		AggQC: aggregateQC,
	}
}

var (
	blockField  *hotstuffpb.Block
	cBlockField *hs.Block
)

func BenchmarkTranslationProto2C(b *testing.B) {
	m := proposeMsgStruct()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proposeMsg := hotstuffpb.ProposalFromProto(m)
		cBlockField = proposeMsg.Block
	}
}

func BenchmarkTranslationC2Proto(b *testing.B) {
	m := proposeMsgStruct()
	proposal := hotstuffpb.ProposalFromProto(m)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proposeMsg := hotstuffpb.ProposalToProto(proposal)
		blockField = proposeMsg.Block
	}
}

func BenchmarkInterface(b *testing.B) {
	m := proposeMsgStruct()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proposeMsg := hotstuffpb.NewProposeMsg(0, m.Block, m.AggQC)
		blockField = proposeMsg.GetBlock()
	}
}

func BenchmarkInterfaceAccess(b *testing.B) {
	m := proposeMsgStruct()
	proposeMsg := hotstuffpb.NewProposeMsg(0, m.Block, m.AggQC)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blockField = proposeMsg.GetBlock()
	}
}

func BenchmarkMethodAccess(b *testing.B) {
	proposeMsg := proposeMsgStruct()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blockField = proposeMsg.GetBlock()
	}
}

func BenchmarkFieldAccess(b *testing.B) {
	proposeMsg := proposeMsgStruct()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blockField = proposeMsg.Block
	}
}
