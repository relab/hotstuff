package twins

import (
	"slices"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/test"
	"github.com/relab/hotstuff/protocol/rules"
)

func TestNetworkSendMessage(t *testing.T) {
	network, senders := createNetwork(t, 4)

	tests := []struct {
		msg      string
		sender   hotstuff.ID
		receiver hotstuff.ID
		want     pendingMessage
	}{
		{msg: "s1r2", sender: 1, receiver: 2, want: pendingMessage{"s1r2", 1, 2}},
		{msg: "s1r3", sender: 1, receiver: 3, want: pendingMessage{"s1r3", 1, 3}},
		{msg: "s1r4", sender: 1, receiver: 4, want: pendingMessage{"s1r4", 1, 4}},
		{msg: "s2r1", sender: 2, receiver: 1, want: pendingMessage{"s2r1", 2, 1}},
		{msg: "s2r3", sender: 2, receiver: 3, want: pendingMessage{"s2r3", 2, 3}},
		{msg: "s2r4", sender: 2, receiver: 4, want: pendingMessage{"s2r4", 2, 4}},
		{msg: "s3r1", sender: 3, receiver: 1, want: pendingMessage{"s3r1", 3, 1}},
		{msg: "s3r2", sender: 3, receiver: 2, want: pendingMessage{"s3r2", 3, 2}},
		{msg: "s3r4", sender: 3, receiver: 4, want: pendingMessage{"s3r4", 3, 4}},
		{msg: "s4r1", sender: 4, receiver: 1, want: pendingMessage{"s4r1", 4, 1}},
		{msg: "s4r2", sender: 4, receiver: 2, want: pendingMessage{"s4r2", 4, 2}},
		{msg: "s4r3", sender: 4, receiver: 3, want: pendingMessage{"s4r3", 4, 3}},
	}
	var wantPendingMessages []pendingMessage
	for _, tt := range tests {
		sender := senders[tt.sender-1]
		sender.sendMessage(tt.receiver, tt.msg)
		checkAppendedMessages(t, tt.msg, network.pendingMessages, []pendingMessage{tt.want})
		wantPendingMessages = append(wantPendingMessages, tt.want)
	}

	gotPendingMessages := slices.Clone(network.pendingMessages)
	checkAllMessages(t, gotPendingMessages, wantPendingMessages)
}

func TestNetworkBroadcastMessage(t *testing.T) {
	network, senders := createNetwork(t, 4)

	tests := []struct {
		msg    string
		sender hotstuff.ID
		want   []pendingMessage
	}{
		{msg: "s1", sender: 1, want: []pendingMessage{{"s1", 1, 2}, {"s1", 1, 3}, {"s1", 1, 4}}}, // receivers: {2, 3, 4}
		{msg: "s2", sender: 2, want: []pendingMessage{{"s2", 2, 1}, {"s2", 2, 3}, {"s2", 2, 4}}}, // receivers: {1, 3, 4}
		{msg: "s3", sender: 3, want: []pendingMessage{{"s3", 3, 1}, {"s3", 3, 2}, {"s3", 3, 4}}}, // receivers: {1, 2, 4}
		{msg: "s4", sender: 4, want: []pendingMessage{{"s4", 4, 1}, {"s4", 4, 2}, {"s4", 4, 3}}}, // receivers: {1, 2, 3}
	}
	var wantPendingMessages []pendingMessage
	for _, tt := range tests {
		sender := senders[tt.sender-1]
		sender.broadcastMessage(tt.msg)
		checkAppendedMessages(t, tt.msg, network.pendingMessages, tt.want)
		wantPendingMessages = append(wantPendingMessages, tt.want...)
	}

	gotPendingMessages := slices.Clone(network.pendingMessages)
	checkAllMessages(t, gotPendingMessages, wantPendingMessages)
}

func TestNetworkSubConfigBroadcastMessage(t *testing.T) {
	_, senders := createNetwork(t, 7)

	t1 := hotstuff.TimeoutMsg{ID: 1, View: 1}
	tests := []struct {
		name      string
		msg       hotstuff.TimeoutMsg
		sender    hotstuff.ID
		receivers []hotstuff.ID
		want      []pendingMessage
	}{
		// sender is also part of the receivers list, but it will not send to itself.
		{name: "self/sender=1/receivers=1", msg: t1, sender: 1, receivers: []hotstuff.ID{1}, want: []pendingMessage{}},
		{name: "self/sender=1/receivers=2", msg: t1, sender: 1, receivers: []hotstuff.ID{1, 2}, want: []pendingMessage{{t1, 1, 2}}},
		{name: "self/sender=1/receivers=3", msg: t1, sender: 1, receivers: []hotstuff.ID{1, 2, 3}, want: []pendingMessage{{t1, 1, 2}, {t1, 1, 3}}},
		{name: "self/sender=1/receivers=4", msg: t1, sender: 1, receivers: []hotstuff.ID{1, 2, 3, 4}, want: []pendingMessage{{t1, 1, 2}, {t1, 1, 3}, {t1, 1, 4}}},
		{name: "self/sender=2/receivers=5", msg: t1, sender: 2, receivers: []hotstuff.ID{1, 2, 3, 4, 5}, want: []pendingMessage{{t1, 2, 1}, {t1, 2, 3}, {t1, 2, 4}, {t1, 2, 5}}},
		{name: "self/sender=3/receivers=6", msg: t1, sender: 3, receivers: []hotstuff.ID{1, 2, 3, 4, 5, 6}, want: []pendingMessage{{t1, 3, 1}, {t1, 3, 2}, {t1, 3, 4}, {t1, 3, 5}, {t1, 3, 6}}},
		{name: "self/sender=4/receivers=7", msg: t1, sender: 4, receivers: []hotstuff.ID{1, 2, 3, 4, 5, 6, 7}, want: []pendingMessage{{t1, 4, 1}, {t1, 4, 2}, {t1, 4, 3}, {t1, 4, 5}, {t1, 4, 6}, {t1, 4, 7}}},
		{name: "self/sender=5/receivers=6", msg: t1, sender: 5, receivers: []hotstuff.ID{2, 3, 4, 5, 6, 7}, want: []pendingMessage{{t1, 5, 2}, {t1, 5, 3}, {t1, 5, 4}, {t1, 5, 6}, {t1, 5, 7}}},
		{name: "self/sender=6/receivers=6", msg: t1, sender: 6, receivers: []hotstuff.ID{1, 3, 4, 5, 6, 7}, want: []pendingMessage{{t1, 6, 1}, {t1, 6, 3}, {t1, 6, 4}, {t1, 6, 5}, {t1, 6, 7}}},
		{name: "self/sender=7/receivers=6", msg: t1, sender: 7, receivers: []hotstuff.ID{1, 2, 4, 5, 6, 7}, want: []pendingMessage{{t1, 7, 1}, {t1, 7, 2}, {t1, 7, 4}, {t1, 7, 5}, {t1, 7, 6}}},
		// sender is not part of the receivers list.
		{name: "no_self/sender=1/receivers=1", msg: t1, sender: 1, receivers: []hotstuff.ID{2}, want: []pendingMessage{{t1, 1, 2}}},
		{name: "no_self/sender=1/receivers=2", msg: t1, sender: 1, receivers: []hotstuff.ID{2, 3}, want: []pendingMessage{{t1, 1, 2}, {t1, 1, 3}}},
		{name: "no_self/sender=1/receivers=3", msg: t1, sender: 1, receivers: []hotstuff.ID{2, 3, 4}, want: []pendingMessage{{t1, 1, 2}, {t1, 1, 3}, {t1, 1, 4}}},
		{name: "no_self/sender=2/receivers=4", msg: t1, sender: 2, receivers: []hotstuff.ID{1, 3, 4, 5}, want: []pendingMessage{{t1, 2, 1}, {t1, 2, 3}, {t1, 2, 4}, {t1, 2, 5}}},
		{name: "no_self/sender=3/receivers=5", msg: t1, sender: 3, receivers: []hotstuff.ID{1, 2, 4, 5, 6}, want: []pendingMessage{{t1, 3, 1}, {t1, 3, 2}, {t1, 3, 4}, {t1, 3, 5}, {t1, 3, 6}}},
		{name: "no_self/sender=4/receivers=6", msg: t1, sender: 4, receivers: []hotstuff.ID{1, 2, 3, 5, 6, 7}, want: []pendingMessage{{t1, 4, 1}, {t1, 4, 2}, {t1, 4, 3}, {t1, 4, 5}, {t1, 4, 6}, {t1, 4, 7}}},
		{name: "no_self/sender=5/receivers=5", msg: t1, sender: 5, receivers: []hotstuff.ID{2, 3, 4, 6, 7}, want: []pendingMessage{{t1, 5, 2}, {t1, 5, 3}, {t1, 5, 4}, {t1, 5, 6}, {t1, 5, 7}}},
		{name: "no_self/sender=6/receivers=5", msg: t1, sender: 6, receivers: []hotstuff.ID{1, 3, 4, 5, 7}, want: []pendingMessage{{t1, 6, 1}, {t1, 6, 3}, {t1, 6, 4}, {t1, 6, 5}, {t1, 6, 7}}},
		{name: "no_self/sender=7/receivers=5", msg: t1, sender: 7, receivers: []hotstuff.ID{1, 2, 4, 5, 6}, want: []pendingMessage{{t1, 7, 1}, {t1, 7, 2}, {t1, 7, 4}, {t1, 7, 5}, {t1, 7, 6}}},
	}
	var gotPendingMessages, wantPendingMessages []pendingMessage
	for _, tt := range tests {
		sender := senders[tt.sender-1]
		sub, err := sender.Sub(tt.receivers)
		if err != nil {
			t.Fatalf("Failed to create subconfiguration: %v", err)
		}
		// using the Timeout call to broadcast to the subconfiguration.
		sub.Timeout(tt.msg)
		// needed to access network.pendingMessages
		network := sub.(*emulatedSender).network
		checkAppendedMessages(t, tt.name, network.pendingMessages, tt.want)
		gotPendingMessages = append(gotPendingMessages, network.pendingMessages...)
		wantPendingMessages = append(wantPendingMessages, tt.want...)
		network.pendingMessages = nil // reset pending messages for the next test
	}

	checkAllMessages(t, gotPendingMessages, wantPendingMessages)
}

func BenchmarkNetworkSubConfigBroadcastMessage(b *testing.B) {
	_, senders := createNetwork(b, 7)

	t1 := hotstuff.TimeoutMsg{ID: 1, View: 1}
	tests := []struct {
		msg       hotstuff.TimeoutMsg
		sender    hotstuff.ID
		receivers []hotstuff.ID
	}{
		// sender is also part of the receivers list, but it will not send to itself.
		{msg: t1, sender: 1, receivers: []hotstuff.ID{1}},
		{msg: t1, sender: 1, receivers: []hotstuff.ID{1, 2}},
		{msg: t1, sender: 1, receivers: []hotstuff.ID{1, 2, 3}},
		{msg: t1, sender: 1, receivers: []hotstuff.ID{1, 2, 3, 4}},
		{msg: t1, sender: 1, receivers: []hotstuff.ID{1, 2, 3, 4, 5}},
		{msg: t1, sender: 1, receivers: []hotstuff.ID{1, 2, 3, 4, 5, 6}},
		{msg: t1, sender: 1, receivers: []hotstuff.ID{1, 2, 3, 4, 5, 6, 7}},
	}
	for _, tt := range tests {
		b.Run(test.Name([]string{"receivers"}, len(tt.receivers)), func(b *testing.B) {
			for b.Loop() {
				sender := senders[tt.sender-1]
				sub, err := sender.Sub(tt.receivers)
				if err != nil {
					b.Fatalf("Failed to create subconfiguration: %v", err)
				}
				// using the Timeout call to broadcast to the subconfiguration.
				sub.Timeout(tt.msg)
				// needed to access network.pendingMessages
				network := sub.(*emulatedSender).network
				network.pendingMessages = nil // reset pending messages for the next test
			}
		})
	}
}

func createNetwork(t testing.TB, numNodes int) (*Network, []*emulatedSender) {
	t.Helper()
	network := NewSimpleNetwork(numNodes)
	// create 4 nodes without twins
	nodes, _ := assignNodeIDs(uint8(numNodes), 0)

	if err := network.createTwinsNodes(nodes, rules.NameChainedHotstuff); err != nil {
		t.Fatalf("Failed to create nodes: %v", err)
	}
	senders := make([]*emulatedSender, len(network.nodes))
	for i, node := range network.nodes {
		senders[i-1] = newSender(network, node)
	}
	return network, senders
}

func checkAppendedMessages(t *testing.T, name string, netMessages, want []pendingMessage) {
	t.Helper()
	if len(netMessages) < len(want) {
		t.Errorf("%s: got %d network messages, want at least %d", name, len(netMessages), len(want))
		return
	}
	// only last len(want) network messages are relevant for this test
	got := slices.Clone(netMessages[len(netMessages)-len(want):])
	slices.SortFunc(got, func(a, b pendingMessage) int {
		if a.sender != b.sender {
			return int(a.sender) - int(b.sender)
		}
		if a.receiver != b.receiver {
			return int(a.receiver) - int(b.receiver)
		}
		return 0
	})
	for i := range got {
		if got[i].sender != want[i].sender || got[i].receiver != want[i].receiver || got[i].message != want[i].message {
			t.Errorf("%s: pendingMessages[%d] = %v, want %v", name, i, got[i], want[i])
		}
	}
}

func checkAllMessages(t *testing.T, got []pendingMessage, want []pendingMessage) {
	t.Helper()
	// Sort the got pending messages for comparison with the want pending messages.
	// This is necessary because broadcasting message iterates over the receivers
	// in a random order, since they are stored in a map (receivers=network.replicas).
	slices.SortFunc(got, func(a, b pendingMessage) int {
		if a.sender != b.sender {
			return int(a.sender) - int(b.sender)
		}
		if a.receiver != b.receiver {
			return int(a.receiver) - int(b.receiver)
		}
		return 0
	})
	// Sort the want pending messages for comparison with the got pending messages.
	// This is necessary because the want pending messages are created in a specific order,
	// which is not guaranteed to be the same as the order as the sorted got pending messages
	// when there are repeated sender-receiver pairs.
	slices.SortFunc(want, func(a, b pendingMessage) int {
		if a.sender != b.sender {
			return int(a.sender) - int(b.sender)
		}
		if a.receiver != b.receiver {
			return int(a.receiver) - int(b.receiver)
		}
		return 0
	})

	for i := range max(len(got), len(want)) {
		if i >= len(got) {
			t.Errorf("pendingMessages[%d] = none, want: %v", i, want[i])
			continue
		}
		if i >= len(want) {
			t.Errorf("pendingMessages[%d] = %v, want none", i, got[i])
			continue
		}
		if got[i].sender != want[i].sender || got[i].receiver != want[i].receiver || got[i].message != want[i].message {
			t.Errorf("pendingMessages[%d] = %v, want %v", i, got[i], want[i])
		}
	}

	if t.Failed() {
		// ignore message content for log clarity
		for i := range got {
			got[i].message = nil
			want[i].message = nil
		}
		t.Logf("pendingMessages:  got %v", got)
		t.Logf("pendingMessages: want %v", want)
	}
}
