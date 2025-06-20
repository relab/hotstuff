package testutil

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/wiring"
)

const cryptoName = ecdsa.ModuleName

// Essentials is a bundle of components essential for constructing simple data for a replica
// in basic consensus protocol unit tests.
type Essentials struct {
	wiring.Core
	wiring.Security
	sender *MockSender
}

func WireUpEssentials(
	t *testing.T,
	id hotstuff.ID,
	cryptoName string,
	opts ...core.RuntimeOption,
) *Essentials {
	t.Helper()
	depsCore := wiring.NewCore(id, "test", GenerateKey(t, cryptoName), opts...)
	sender := NewMockSender(id)
	depsSecurity, err := wiring.NewSecurity(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		sender,
		cryptoName,
	)
	if err != nil {
		t.Fatal(err)
	}
	return &Essentials{
		Core:     *depsCore, // no problem dereferencing, since the deps just hold pointers
		Security: *depsSecurity,
		sender:   sender,
	}
}

func (e *Essentials) MockSender() *MockSender {
	return e.sender
}

func WireUpSigners(t *testing.T, parent *Essentials, count uint, cryptoName string) []*cert.Authority {
	t.Helper()
	if count == 0 {
		t.Fatal("signer count cannot be zero")
	}
	dummies := make([]*Essentials, 0)
	replicas := make([]hotstuff.ReplicaInfo, 0)
	signers := make([]*cert.Authority, 0)
	signers = append(signers, parent.Authority())
	for i := range count {
		id := hotstuff.ID(i + 2)
		dummy := WireUpEssentials(t, id, cryptoName)
		replicas = append(replicas, hotstuff.ReplicaInfo{
			ID:       hotstuff.ID(id + 1),
			PubKey:   dummy.RuntimeCfg().PrivateKey().Public(),
			Metadata: dummy.RuntimeCfg().ConnectionMetadata(),
		})
		signers = append(signers, dummy.Authority())
	}
	for _, dummy := range dummies {
		for _, replica := range replicas {
			dummy.RuntimeCfg().AddReplica(&replica)
		}
	}
	return signers
}
