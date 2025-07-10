package testutil

import (
	"context"
	"fmt"
	"slices"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/security/blockchain"
)

type MockSender struct {
	id           hotstuff.ID
	recipients   []hotstuff.ID
	messagesSent []any
	blockChains  []*blockchain.Blockchain
}

// NewMockSender returns a mock implementation of core.Sender that
// just stores messages in a slice to make testing easier.
// Optionally, the mock sender can be restricted to send to a specific
// set of recipients.
func NewMockSender(id hotstuff.ID, recipients ...hotstuff.ID) *MockSender {
	return &MockSender{
		id:         id,
		recipients: recipients,
	}
}

// MessagesSent returns a slice of messages that were sent by the mock sender.
func (m *MockSender) MessagesSent() []any {
	return m.messagesSent
}

// AddBlockchain saves a reference to a blockchain storage from a separate mock replica.
// This is necessary for block lookup by RequestBlock.
func (m *MockSender) AddBlockchain(chain *blockchain.Blockchain) {
	if chain == nil {
		panic("blockchain pointer cannot be nil")
	}
	m.blockChains = append(m.blockChains, chain)
}

// saveUnicast stores the message sent to one recipient by this mock sender.
// If m.recipients is not empty, the message will be discarded if the recipient was not in m.recipients.
func (m *MockSender) saveUnicast(recipient hotstuff.ID, msg any) {
	if len(m.recipients) > 0 && !slices.Contains(m.recipients, recipient) {
		return
	}
	m.messagesSent = append(m.messagesSent, msg)
}

// saveBroadcast stores the message broadcasted by this mock sender.
// NOTE: it just wraps saveUnicast for now.
func (m *MockSender) saveBroadcast(msg any) {
	m.saveUnicast(m.id, msg)
}

// NewView mock that stores msg.
func (m *MockSender) NewView(id hotstuff.ID, msg hotstuff.SyncInfo) error {
	m.saveUnicast(id, msg)
	return nil
}

// Vote mock that stores cert.
func (m *MockSender) Vote(id hotstuff.ID, cert hotstuff.PartialCert) error {
	m.saveUnicast(id, cert)
	return nil
}

// Timeout mock that stores msg.
func (m *MockSender) Timeout(msg hotstuff.TimeoutMsg) {
	m.saveBroadcast(msg)
}

// Propose mock that stores the value of proposal.
func (m *MockSender) Propose(proposal *hotstuff.ProposeMsg) {
	m.saveBroadcast(*proposal)
}

// RequestBlock mock traverses the known blockchain references added by AddBlockchain and
// attempts to retrieve the block by each chain's LocalGet method.
// NOTE: this method panics if AddBlockchain was not called at least once.
func (m *MockSender) RequestBlock(_ context.Context, hash hotstuff.Hash) (*hotstuff.Block, bool) {
	if len(m.blockChains) == 0 {
		panic("no blockchains available to mock sender")
	}
	for _, chain := range m.blockChains {
		block, ok := chain.LocalGet(hash)
		if ok {
			return block, true
		}
	}
	return nil, false
}

// Sub mock returns a copy MockSender that only allows to send to the provided sender ID's.
// The method returns an error if the ids are not a subset of m.recipients.
func (m *MockSender) Sub(ids []hotstuff.ID) (core.Sender, error) {
	if len(m.recipients) < len(ids) {
		return nil, fmt.Errorf("cannot return a sub sender when ids slice is larger")
	}
	// checks if ids intersects m.recipients
	if !isSubset(m.recipients, ids) {
		return nil, fmt.Errorf("one or more ids are not a subset of the parent's recipients")
	}
	return &MockSender{
		id:         m.id,
		recipients: ids,
	}, nil
}

var _ core.Sender = (*MockSender)(nil)

func isSubset(a, b []hotstuff.ID) bool {
	set := make(map[hotstuff.ID]struct{}, len(a))
	for _, id := range a {
		set[id] = struct{}{}
	}
	for _, id := range b {
		if _, found := set[id]; !found {
			return false
		}
	}
	return true
}
