package testutil

import (
	"context"
	"slices"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
)

// type MockMessage struct {
// 	Sender    hotstuff.ID
// 	Recipient hotstuff.ID
// 	Value     any
// }

type MockSender struct {
	id                  hotstuff.ID
	availableRecipients []hotstuff.ID
	messagesSent        []any
}

func NewMockSender(id hotstuff.ID) *MockSender {
	return &MockSender{
		id: id,
	}
}

func (m *MockSender) MessagesSent() []any {
	return m.messagesSent
}

func (m *MockSender) saveUnicast(recipient hotstuff.ID, msg any) {
	if m.availableRecipients != nil && !slices.Contains(m.availableRecipients, recipient) {
		return
	}
	m.messagesSent = append(m.messagesSent, msg)
}

func (m *MockSender) saveBroadcast(msg any) {
	// if m.availableRecipients != nil {
	// 	for _, recipient := range m.availableRecipients {
	// 		if recipient == m.id {
	// 			continue
	// 		}
	// 		m.messagesSent = append(m.messagesSent, MockMessage{
	// 			Sender:    m.id,
	// 			Recipient: recipient,
	// 			Value:     msg,
	// 		})
	// 	}
	// 	return
	// }
	// for now, it just sends to self since i'm not sure about how broadcast should be handled
	m.saveUnicast(m.id, msg)
}

func (m *MockSender) NewView(id hotstuff.ID, msg hotstuff.SyncInfo) error {
	m.saveUnicast(id, msg)
	return nil
}

func (m *MockSender) Vote(id hotstuff.ID, cert hotstuff.PartialCert) error {
	// Simulate sending a vote
	m.saveUnicast(id, cert)
	return nil
}

func (m *MockSender) Timeout(msg hotstuff.TimeoutMsg) {
	m.saveBroadcast(msg)
}

func (m *MockSender) Propose(proposal *hotstuff.ProposeMsg) {
	m.saveBroadcast(*proposal)
}

func (m *MockSender) RequestBlock(_ context.Context, _ hotstuff.Hash) (*hotstuff.Block, bool) {
	panic("not implemented")
}

func (m *MockSender) Sub(ids []hotstuff.ID) (modules.Sender, error) {
	// Simulate creating a sub-sender for specific IDs
	return &MockSender{
		id:                  m.id,
		availableRecipients: ids,
	}, nil
}

var _ modules.Sender = (*MockSender)(nil)
