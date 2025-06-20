package testutil

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
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
