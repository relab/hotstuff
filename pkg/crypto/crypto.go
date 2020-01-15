package crypto

import (
	"strconv"

	"github.com/relab/hotstuff/pkg/proto"
)

type PartialSig string

func (p PartialSig) String() string {
	return string(p)
}

type ThresholdSig string

func (t ThresholdSig) String() string {
	return string(t)
}

type Crypto interface {
	Sign(*proto.Msg) PartialSig
	Verify(*proto.QuorumCert) bool
	Combine(proto.Type, int32, *proto.HSNode, []*proto.Msg) ThresholdSig
}

type NoCrypto struct{}

func (c NoCrypto) Sign(msg *proto.Msg) PartialSig {
	msg.PartialSig = "hotstuff"
}

func (c NoCrypto) Combine(t proto.Type, vn int32, n *proto.HSNode, msgs []*proto.Msg) ThresholdSig {
	return ThresholdSig(strconv.Itoa(len(msgs)) + "hotstuff")
}

func (c NoCrypto) Verify(cert *proto.QuorumCert) bool {
	return true
}
