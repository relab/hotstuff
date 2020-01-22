package hotstuff

import (
	"crypto/ecdsa"
	"crypto/rand"
	"math/big"
)

type PartialSig struct {
	ID   ReplicaID
	R, S *big.Int
}

type QuorumSig []*PartialSig

func CreatePartialSig(id ReplicaID, privKey *ecdsa.PrivateKey, hash []byte) (*PartialSig, error) {
	r, s, err := ecdsa.Sign(rand.Reader, privKey, hash)
	if err != nil {
		return nil, err
	}
	return &PartialSig{ ID: id, R: r, S: s }, nil
}

func VerifyPartialSig(conf ReplicaConfig, hash []byte, ps *PartialSig) bool {
	info, ok := conf.Replicas[ps.ID]
	if !ok {
		logger.Printf("VerifyPartialSig: got signature from replica whose ID (%d) was not in config.", ps.ID)
		return false
	}
	return ecdsa.Verify(info.PubKey, hash, ps.R, ps.S)
}

func VerifyQuorumSig(conf ReplicaConfig, hash []byte, qs QuorumSig) bool {
	numVerified := 0
	for _, sig := range qs {
		info, ok := conf.Replicas[sig.ID]
		if !ok {
			logger.Printf("VerifyQuorumSig: got signature from replica whose ID (%d) was not in config.", sig.ID)
		}

		if ecdsa.Verify(info.PubKey, hash, sig.R, sig.S) {
			numVerified++
		}
	}
	return numVerified >= conf.Majority
}
