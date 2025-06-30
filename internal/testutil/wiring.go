package testutil

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto"
	"github.com/relab/hotstuff/wiring"
)

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
	opts ...cert.Option,
) *Essentials {
	t.Helper()
	// NOTE: using synchronous vote verification to keep tests simple.
	depsCore := wiring.NewCore(id, "test", GenerateKey(t, cryptoName), core.WithSyncVerification())
	sender := NewMockSender(id)
	base, err := crypto.New(
		depsCore.RuntimeCfg(),
		cryptoName,
	)
	if err != nil {
		t.Fatal(err)
	}
	depsSecurity := wiring.NewSecurity(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		sender,
		base,
		opts...,
	)
	return &Essentials{
		Core:     *depsCore, // no problem dereferencing, since the deps just hold pointers
		Security: *depsSecurity,
		sender:   sender,
	}
}

func (e *Essentials) MockSender() *MockSender {
	return e.sender
}

type EssentialsSet []*Essentials

// NewEssentialsSet wires up multiple essential component bundles and adds each replica configuration
// to each other.
func NewEssentialsSet(
	t *testing.T,
	count uint,
	cryptoName string,
	opts ...cert.Option,
) EssentialsSet {
	t.Helper()
	if count == 0 {
		t.Fatal("signer count cannot be zero")
	}
	dummies := make([]*Essentials, 0)
	replicas := make([]hotstuff.ReplicaInfo, 0)
	for i := range count {
		id := hotstuff.ID(i + 1)
		dummy := WireUpEssentials(t, id, cryptoName, opts...)
		replicas = append(replicas, hotstuff.ReplicaInfo{
			ID:       hotstuff.ID(id),
			PubKey:   dummy.RuntimeCfg().PrivateKey().Public(),
			Metadata: dummy.RuntimeCfg().ConnectionMetadata(),
		})
		dummies = append(dummies, dummy)
	}
	for _, dummy := range dummies {
		for _, replica := range replicas {
			dummy.RuntimeCfg().AddReplica(&replica)
		}
		for _, other := range dummies {
			if other == dummy {
				continue
			}
			dummy.MockSender().AddBlockchain(other.Blockchain())
		}
	}
	return dummies
}

func (s EssentialsSet) Signers() []*cert.Authority {
	signers := make([]*cert.Authority, 0)
	for _, dummy := range s {
		signers = append(signers, dummy.Authority())
	}
	return signers
}
