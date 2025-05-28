package testutil

import (
	"context"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
)

type MockSender struct {
	id                hotstuff.ID
	network           *MockNetwork
	replicasAvailable []hotstuff.ID
}

func NewMockSender(
	id hotstuff.ID,
	network *MockNetwork,
) *MockSender {
	return &MockSender{
		id:      id,
		network: network,
	}
}

func (m *MockSender) NewView(id hotstuff.ID, msg hotstuff.SyncInfo) error {
	// Simulate sending a NewView message
	m.network.unicast(id, m.replicasAvailable, msg)
	return nil
}

func (m *MockSender) Vote(id hotstuff.ID, cert hotstuff.PartialCert) error {
	// Simulate sending a vote
	m.network.unicast(id, m.replicasAvailable, cert)
	return nil
}

func (m *MockSender) Timeout(msg hotstuff.TimeoutMsg) {
	// Simulate sending a timeout message
	m.network.broadcast(m.id, m.replicasAvailable, msg)
}

func (m *MockSender) Propose(proposal *hotstuff.ProposeMsg) {
	// Simulate sending a proposal
	m.network.broadcast(m.id, m.replicasAvailable, *proposal)
}

func (m *MockSender) RequestBlock(_ context.Context, hash hotstuff.Hash) (*hotstuff.Block, bool) {
	// Simulate block request
	for _, replica := range m.network.replicas {
		block, ok := replica.BlockChain.LocalGet(hash)
		if ok {
			return block, true
		}
	}
	return nil, false
}

func (m *MockSender) Sub(ids []hotstuff.ID) (modules.Sender, error) {
	// Simulate creating a sub-sender for specific IDs
	return &MockSender{
		id:                m.id,
		network:           m.network,
		replicasAvailable: ids,
	}, nil
}

var _ modules.Sender = (*MockSender)(nil)
