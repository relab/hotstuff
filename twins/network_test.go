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
		{msg: "s1r2", sender: 1, receiver: 2, want: pendingMessage{"s1r2", Replica(1), Replica(2), 1}},
		{msg: "s1r3", sender: 1, receiver: 3, want: pendingMessage{"s1r3", Replica(1), Replica(3), 1}},
		{msg: "s1r4", sender: 1, receiver: 4, want: pendingMessage{"s1r4", Replica(1), Replica(4), 1}},
		{msg: "s2r1", sender: 2, receiver: 1, want: pendingMessage{"s2r1", Replica(2), Replica(1), 1}},
		{msg: "s2r3", sender: 2, receiver: 3, want: pendingMessage{"s2r3", Replica(2), Replica(3), 1}},
		{msg: "s2r4", sender: 2, receiver: 4, want: pendingMessage{"s2r4", Replica(2), Replica(4), 1}},
		{msg: "s3r1", sender: 3, receiver: 1, want: pendingMessage{"s3r1", Replica(3), Replica(1), 1}},
		{msg: "s3r2", sender: 3, receiver: 2, want: pendingMessage{"s3r2", Replica(3), Replica(2), 1}},
		{msg: "s3r4", sender: 3, receiver: 4, want: pendingMessage{"s3r4", Replica(3), Replica(4), 1}},
		{msg: "s4r1", sender: 4, receiver: 1, want: pendingMessage{"s4r1", Replica(4), Replica(1), 1}},
		{msg: "s4r2", sender: 4, receiver: 2, want: pendingMessage{"s4r2", Replica(4), Replica(2), 1}},
		{msg: "s4r3", sender: 4, receiver: 3, want: pendingMessage{"s4r3", Replica(4), Replica(3), 1}},
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
		{msg: "s1", sender: 1, want: []pendingMessage{{"s1", Replica(1), Replica(2), 1}, {"s1", Replica(1), Replica(3), 1}, {"s1", Replica(1), Replica(4), 1}}}, // receivers: {2, 3, 4}
		{msg: "s2", sender: 2, want: []pendingMessage{{"s2", Replica(2), Replica(1), 1}, {"s2", Replica(2), Replica(3), 1}, {"s2", Replica(2), Replica(4), 1}}}, // receivers: {1, 3, 4}
		{msg: "s3", sender: 3, want: []pendingMessage{{"s3", Replica(3), Replica(1), 1}, {"s3", Replica(3), Replica(2), 1}, {"s3", Replica(3), Replica(4), 1}}}, // receivers: {1, 2, 4}
		{msg: "s4", sender: 4, want: []pendingMessage{{"s4", Replica(4), Replica(1), 1}, {"s4", Replica(4), Replica(2), 1}, {"s4", Replica(4), Replica(3), 1}}}, // receivers: {1, 2, 3}
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
		{name: "self/sender=1/receivers=2", msg: t1, sender: 1, receivers: []hotstuff.ID{1, 2}, want: []pendingMessage{{t1, Replica(1), Replica(2), 1}}},
		{name: "self/sender=1/receivers=3", msg: t1, sender: 1, receivers: []hotstuff.ID{1, 2, 3}, want: []pendingMessage{{t1, Replica(1), Replica(2), 1}, {t1, Replica(1), Replica(3), 1}}},
		{name: "self/sender=1/receivers=4", msg: t1, sender: 1, receivers: []hotstuff.ID{1, 2, 3, 4}, want: []pendingMessage{{t1, Replica(1), Replica(2), 1}, {t1, Replica(1), Replica(3), 1}, {t1, Replica(1), Replica(4), 1}}},
		{name: "self/sender=2/receivers=5", msg: t1, sender: 2, receivers: []hotstuff.ID{1, 2, 3, 4, 5}, want: []pendingMessage{{t1, Replica(2), Replica(1), 1}, {t1, Replica(2), Replica(3), 1}, {t1, Replica(2), Replica(4), 1}, {t1, Replica(2), Replica(5), 1}}},
		{name: "self/sender=3/receivers=6", msg: t1, sender: 3, receivers: []hotstuff.ID{1, 2, 3, 4, 5, 6}, want: []pendingMessage{{t1, Replica(3), Replica(1), 1}, {t1, Replica(3), Replica(2), 1}, {t1, Replica(3), Replica(4), 1}, {t1, Replica(3), Replica(5), 1}, {t1, Replica(3), Replica(6), 1}}},
		{name: "self/sender=4/receivers=7", msg: t1, sender: 4, receivers: []hotstuff.ID{1, 2, 3, 4, 5, 6, 7}, want: []pendingMessage{{t1, Replica(4), Replica(1), 1}, {t1, Replica(4), Replica(2), 1}, {t1, Replica(4), Replica(3), 1}, {t1, Replica(4), Replica(5), 1}, {t1, Replica(4), Replica(6), 1}, {t1, Replica(4), Replica(7), 1}}},
		{name: "self/sender=5/receivers=6", msg: t1, sender: 5, receivers: []hotstuff.ID{2, 3, 4, 5, 6, 7}, want: []pendingMessage{{t1, Replica(5), Replica(2), 1}, {t1, Replica(5), Replica(3), 1}, {t1, Replica(5), Replica(4), 1}, {t1, Replica(5), Replica(6), 1}, {t1, Replica(5), Replica(7), 1}}},
		{name: "self/sender=6/receivers=6", msg: t1, sender: 6, receivers: []hotstuff.ID{1, 3, 4, 5, 6, 7}, want: []pendingMessage{{t1, Replica(6), Replica(1), 1}, {t1, Replica(6), Replica(3), 1}, {t1, Replica(6), Replica(4), 1}, {t1, Replica(6), Replica(5), 1}, {t1, Replica(6), Replica(7), 1}}},
		{name: "self/sender=7/receivers=6", msg: t1, sender: 7, receivers: []hotstuff.ID{1, 2, 4, 5, 6, 7}, want: []pendingMessage{{t1, Replica(7), Replica(1), 1}, {t1, Replica(7), Replica(2), 1}, {t1, Replica(7), Replica(4), 1}, {t1, Replica(7), Replica(5), 1}, {t1, Replica(7), Replica(6), 1}}},
		// sender is not part of the receivers list.
		{name: "no_self/sender=1/receivers=1", msg: t1, sender: 1, receivers: []hotstuff.ID{2}, want: []pendingMessage{{t1, Replica(1), Replica(2), 1}}},
		{name: "no_self/sender=1/receivers=2", msg: t1, sender: 1, receivers: []hotstuff.ID{2, 3}, want: []pendingMessage{{t1, Replica(1), Replica(2), 1}, {t1, Replica(1), Replica(3), 1}}},
		{name: "no_self/sender=1/receivers=3", msg: t1, sender: 1, receivers: []hotstuff.ID{2, 3, 4}, want: []pendingMessage{{t1, Replica(1), Replica(2), 1}, {t1, Replica(1), Replica(3), 1}, {t1, Replica(1), Replica(4), 1}}},
		{name: "no_self/sender=2/receivers=4", msg: t1, sender: 2, receivers: []hotstuff.ID{1, 3, 4, 5}, want: []pendingMessage{{t1, Replica(2), Replica(1), 1}, {t1, Replica(2), Replica(3), 1}, {t1, Replica(2), Replica(4), 1}, {t1, Replica(2), Replica(5), 1}}},
		{name: "no_self/sender=3/receivers=5", msg: t1, sender: 3, receivers: []hotstuff.ID{1, 2, 4, 5, 6}, want: []pendingMessage{{t1, Replica(3), Replica(1), 1}, {t1, Replica(3), Replica(2), 1}, {t1, Replica(3), Replica(4), 1}, {t1, Replica(3), Replica(5), 1}, {t1, Replica(3), Replica(6), 1}}},
		{name: "no_self/sender=4/receivers=6", msg: t1, sender: 4, receivers: []hotstuff.ID{1, 2, 3, 5, 6, 7}, want: []pendingMessage{{t1, Replica(4), Replica(1), 1}, {t1, Replica(4), Replica(2), 1}, {t1, Replica(4), Replica(3), 1}, {t1, Replica(4), Replica(5), 1}, {t1, Replica(4), Replica(6), 1}, {t1, Replica(4), Replica(7), 1}}},
		{name: "no_self/sender=5/receivers=5", msg: t1, sender: 5, receivers: []hotstuff.ID{2, 3, 4, 6, 7}, want: []pendingMessage{{t1, Replica(5), Replica(2), 1}, {t1, Replica(5), Replica(3), 1}, {t1, Replica(5), Replica(4), 1}, {t1, Replica(5), Replica(6), 1}, {t1, Replica(5), Replica(7), 1}}},
		{name: "no_self/sender=6/receivers=5", msg: t1, sender: 6, receivers: []hotstuff.ID{1, 3, 4, 5, 7}, want: []pendingMessage{{t1, Replica(6), Replica(1), 1}, {t1, Replica(6), Replica(3), 1}, {t1, Replica(6), Replica(4), 1}, {t1, Replica(6), Replica(5), 1}, {t1, Replica(6), Replica(7), 1}}},
		{name: "no_self/sender=7/receivers=5", msg: t1, sender: 7, receivers: []hotstuff.ID{1, 2, 4, 5, 6}, want: []pendingMessage{{t1, Replica(7), Replica(1), 1}, {t1, Replica(7), Replica(2), 1}, {t1, Replica(7), Replica(4), 1}, {t1, Replica(7), Replica(5), 1}, {t1, Replica(7), Replica(6), 1}}},
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
		b.Run(test.Name("receivers", len(tt.receivers)), func(b *testing.B) {
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
	// create nodes without twins
	nodes, _ := assignNodeIDs(uint8(numNodes), 0)

	if err := network.createNodesAndTwins(nodes, rules.NameChainedHotStuff); err != nil {
		t.Fatalf("Failed to create nodes: %v", err)
	}
	senders := make([]*emulatedSender, numNodes)
	for i, nodeID := range nodes {
		node := network.nodes[nodeID]
		senders[i] = newSender(network, node)
	}
	return network, senders
}

func createNetworkWithTwins(t testing.TB, numNodes, numTwins int) (*Network, []*emulatedSender, []NodeID, []NodeID) {
	t.Helper()
	totalNodes := numNodes + numTwins
	network := NewSimpleNetwork(totalNodes)

	// create nodes with twins
	regularNodes, twinNodes := assignNodeIDs(uint8(numNodes), uint8(numTwins))

	// Combine all nodes (regular + twins) for network creation
	allNodes := append(regularNodes, twinNodes...)

	if err := network.createNodesAndTwins(allNodes, rules.NameChainedHotStuff); err != nil {
		t.Fatalf("Failed to create nodes: %v", err)
	}

	senders := make([]*emulatedSender, len(allNodes))
	for i, nodeID := range allNodes {
		node := network.nodes[nodeID]
		senders[i] = newSender(network, node)
	}
	return network, senders, allNodes, twinNodes
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
		if a.sender.ReplicaID != b.sender.ReplicaID {
			return int(a.sender.ReplicaID) - int(b.sender.ReplicaID)
		}
		if a.receiver.ReplicaID != b.receiver.ReplicaID {
			return int(a.receiver.ReplicaID) - int(b.receiver.ReplicaID)
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
		if a.sender.ReplicaID != b.sender.ReplicaID {
			return int(a.sender.ReplicaID) - int(b.sender.ReplicaID)
		}
		if a.receiver.ReplicaID != b.receiver.ReplicaID {
			return int(a.receiver.ReplicaID) - int(b.receiver.ReplicaID)
		}
		return 0
	})
	// Sort the want pending messages for comparison with the got pending messages.
	// This is necessary because the want pending messages are created in a specific order,
	// which is not guaranteed to be the same as the order as the sorted got pending messages
	// when there are repeated sender-receiver pairs.
	slices.SortFunc(want, func(a, b pendingMessage) int {
		if a.sender.ReplicaID != b.sender.ReplicaID {
			return int(a.sender.ReplicaID) - int(b.sender.ReplicaID)
		}
		if a.receiver.ReplicaID != b.receiver.ReplicaID {
			return int(a.receiver.ReplicaID) - int(b.receiver.ReplicaID)
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

func TestNetworkWithTwins(t *testing.T) {

	// Create a network with 1 twin and 3 hotstuff ids
	network, senders, _, _ := createNetworkWithTwins(t, 3, 1)

	nonTwinSender := senders[0] // index 0 corresponds to r2n0 (first regular node)

	// Clear any existing pending messages
	network.pendingMessages = nil

	// Send a message to the twins' HotStuff ID (ReplicaID)
	testMessage := "message-to-twins"
	nonTwinSender.sendMessage(hotstuff.ID(1), testMessage)

	// Define expected messages: one to each twin (r1n1 and r1n2)
	expectedMessages := []pendingMessage{
		{testMessage, Replica(2), Replica(1).Twin(1), 1},
		{testMessage, Replica(2), Replica(1).Twin(2), 1},
	}

	// Use the helper function to check messages
	checkAllMessages(t, network.pendingMessages, expectedMessages)

	t.Logf("✓ Message successfully sent to both twins: r1n1 and r1n2")
}
