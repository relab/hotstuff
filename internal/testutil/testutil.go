// Package testutil provides helper methods that are useful for implementing tests.
package testutil

import (
	"net"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/mocks"
)

// CreateMockReplica returns a mock of a hotstuff.Replica.
func CreateMockReplica(t *testing.T, ctrl *gomock.Controller, id hotstuff.ID, key hotstuff.PublicKey) *mocks.MockReplica {
	t.Helper()

	replica := mocks.NewMockReplica(ctrl)
	replica.
		EXPECT().
		ID().
		AnyTimes().
		Return(id)
	replica.
		EXPECT().
		PublicKey().
		AnyTimes().
		Return(key)

	return replica
}

// CreateMockConfig returns a mock of a hotstuff.Config.
func CreateMockConfig(t *testing.T, ctrl *gomock.Controller, id hotstuff.ID, key hotstuff.PrivateKey) *mocks.MockConfig {
	t.Helper()

	cfg := mocks.NewMockConfig(ctrl)
	cfg.
		EXPECT().
		PrivateKey().
		AnyTimes().
		Return(key)
	cfg.
		EXPECT().
		ID().
		AnyTimes().
		Return(id)

	return cfg
}

// ConfigAddReplica adds a mock replica to a mock configuration.
func ConfigAddReplica(t *testing.T, cfg *mocks.MockConfig, replica *mocks.MockReplica) {
	t.Helper()

	cfg.
		EXPECT().
		Replica(replica.ID()).
		AnyTimes().
		Return(replica, true)
}

// CreateTCPListener creates a net.Listener on a random port.
func CreateTCPListener(t *testing.T) net.Listener {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	return lis
}
