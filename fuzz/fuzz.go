package fuzz

import (
	"math/rand"

	fuzz "github.com/google/gofuzz"
	hotstuffpb "github.com/relab/hotstuff/internal/proto/hotstuffpb"
)

func initFuzz() *fuzz.Fuzzer {
	nilChance := 0.1

	f := fuzz.New().NilChance(nilChance).Funcs(
		func(m *FuzzMsg, c fuzz.Continue) {
			switch c.Intn(4) {
			case 0:
				msg := FuzzMsg_ProposeMsg{
					ProposeMsg: &ProposeMsg{},
				}
				c.Fuzz(msg.ProposeMsg)
				m.Message = &msg
			case 1:
				msg := FuzzMsg_VoteMsg{
					VoteMsg: &VoteMsg{},
				}
				c.Fuzz(msg.VoteMsg)
				m.Message = &msg
			case 2:
				msg := FuzzMsg_TimeoutMsg{
					TimeoutMsg: &TimeoutMsg{},
				}
				c.Fuzz(msg.TimeoutMsg)
				m.Message = &msg
			case 3:
				msg := FuzzMsg_NewViewMsg{
					NewViewMsg: &NewViewMsg{},
				}
				c.Fuzz(msg.NewViewMsg)
				m.Message = &msg
			}
		},
		func(sig **hotstuffpb.QuorumSignature, c fuzz.Continue) {
			if c.Float64() < nilChance {
				*sig = nil
				return
			}

			*sig = new(hotstuffpb.QuorumSignature)
			switch c.Intn(2) {
			case 0:
				ecdsa := new(hotstuffpb.QuorumSignature_ECDSASigs)
				c.Fuzz(&ecdsa)
				(*sig).Sig = ecdsa
			case 1:
				bls12 := new(hotstuffpb.QuorumSignature_BLS12Sig)
				c.Fuzz(&bls12)
				(*sig).Sig = bls12
			}
		},
	)

	return f
}

func createFuzzMessage(f *fuzz.Fuzzer, seed *int64) *FuzzMsg {

	if seed != nil {
		f.RandSource(rand.NewSource(*seed))
	}

	newMessage := new(FuzzMsg)
	f.Fuzz(newMessage)
	return newMessage
}
