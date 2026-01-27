package comm_test

import (
	"context"
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/test"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/security/crypto"
)

// fullTreeSize calculates the number of nodes in a full tree with the given
// branch factor and 3 levels (root, intermediate, leaves).
func fullTreeSize(bf int) int {
	return 1 + bf + bf*bf // level 0 + level 1 + level 2
}

// treeConfig represents a tree configuration for table-driven tests.
type treeConfig struct {
	branchFactor int
	size         int
}

// standardTreeConfigs returns tree configurations for full 3-level trees
// with branch factors 2, 3, 4, and 5.
func standardTreeConfigs() []treeConfig {
	return []treeConfig{
		{branchFactor: 2, size: fullTreeSize(2)}, // 7 nodes
		{branchFactor: 3, size: fullTreeSize(3)}, // 13 nodes
		{branchFactor: 4, size: fullTreeSize(4)}, // 21 nodes
		{branchFactor: 5, size: fullTreeSize(5)}, // 31 nodes
	}
}

// kauriTestSetup contains all components needed for Kauri tests.
type kauriTestSetup struct {
	essentials *testutil.Essentials
	kauri      *comm.Kauri
	tree       *tree.Tree
	sender     *testutil.MockSender
}

// wireUpKauri creates a Kauri instance with the specified configuration.
// It returns a setup struct containing the Kauri instance and related components.
func wireUpKauri(
	t testing.TB,
	id hotstuff.ID,
	treeSize, branchFactor int,
) *kauriTestSetup {
	t.Helper()

	treePositions := tree.DefaultTreePos(treeSize)
	tr := tree.NewSimple(id, branchFactor, treePositions)
	tr.SetTreeHeightWaitTime(0) // disable wait time for tests

	essentials := testutil.WireUpEssentials(
		t, id, crypto.NameECDSA,
		core.WithKauriTree(tr),
	)

	// Add all replicas to the configuration
	for _, pos := range treePositions {
		if pos != id {
			essentials.RuntimeCfg().AddReplica(&hotstuff.ReplicaInfo{ID: pos})
		}
	}

	// Create a mock sender with all replica IDs as potential recipients
	sender := testutil.NewMockSender(id, treePositions...)

	k := comm.NewKauri(
		essentials.Logger(),
		essentials.EventLoop(),
		essentials.RuntimeCfg(),
		essentials.Blockchain(),
		essentials.Authority(),
		sender,
	)

	return &kauriTestSetup{
		essentials: essentials,
		kauri:      k,
		tree:       tr,
		sender:     sender,
	}
}

// simulateConnection simulates the replica being connected by triggering
// the ReplicaConnectedEvent through the event loop.
func simulateConnection(t testing.TB, setup *kauriTestSetup) {
	t.Helper()
	el := setup.essentials.EventLoop()
	el.AddEvent(hotstuff.ReplicaConnectedEvent{})
	el.Tick(context.Background())
}

// createProposal creates a test proposal with a block based on genesis.
func createProposal(t testing.TB, essentials *testutil.Essentials, proposerID hotstuff.ID) *hotstuff.ProposeMsg {
	t.Helper()
	block := testutil.CreateBlock(t, essentials.Authority())
	return &hotstuff.ProposeMsg{
		ID:    proposerID,
		Block: block,
	}
}

// waitForEvent registers a handler for event type T and ticks the event loop
// until the event is received or timeout occurs. This is preferred over
// arbitrary tick counts.
func waitForEvent[T any](t testing.TB, el *eventloop.EventLoop, timeout time.Duration) {
	t.Helper()
	received := make(chan struct{})
	unregister := eventloop.Register(el, func(_ T) {
		close(received)
	})
	defer unregister()

	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-received:
			return
		default:
			if time.Now().After(deadline) {
				t.Fatalf("timeout waiting for event %T", *new(T))
			}
			el.Tick(context.Background())
			time.Sleep(time.Millisecond) // yield to allow goroutines to run
		}
	}
}

// TestNewKauriPanic tests that NewKauri panics when no tree is configured.
func TestNewKauriPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewKauri should panic when no tree is configured")
		}
	}()

	essentials := testutil.WireUpEssentials(t, 1, crypto.NameECDSA)
	// This should panic because no tree is configured
	_ = comm.NewKauri(
		essentials.Logger(),
		essentials.EventLoop(),
		essentials.RuntimeCfg(),
		essentials.Blockchain(),
		essentials.Authority(),
		essentials.MockSender(),
	)
}

// TestDisseminateFromLeaf tests that a leaf node sends its contribution
// directly to the parent without forwarding to children.
func TestDisseminateFromLeaf(t *testing.T) {
	for _, tc := range standardTreeConfigs() {
		t.Run(test.Name("bf", tc.branchFactor, "size", tc.size), func(t *testing.T) {
			// Use the last node (a leaf) as the test replica
			leafID := hotstuff.ID(tc.size)
			setup := wireUpKauri(t, leafID, tc.size, tc.branchFactor)
			simulateConnection(t, setup)

			proposal := createProposal(t, setup.essentials, 1)
			setup.essentials.Blockchain().Store(proposal.Block)
			pc := testutil.CreatePC(t, proposal.Block, setup.essentials.Authority())

			err := setup.kauri.Disseminate(proposal, pc)
			if err != nil {
				t.Fatalf("Disseminate failed: %v", err)
			}

			// Leaf should send contribution to parent, not propose to children
			contributions := setup.sender.ContributionsSent()
			if len(contributions) != 1 {
				t.Errorf("expected 1 contribution from leaf, got %d", len(contributions))
			}

			// Verify no proposals were sent (leaf has no children)
			messages := setup.sender.MessagesSent()
			for _, msg := range messages {
				if _, ok := msg.(hotstuff.ProposeMsg); ok {
					t.Error("leaf node should not send proposals")
				}
			}
		})
	}
}

// TestDisseminateFromIntermediate tests that an intermediate node with children
// starts the aggregation timer and eventually sends its contribution to parent.
func TestDisseminateFromIntermediate(t *testing.T) {
	for _, tc := range standardTreeConfigs() {
		t.Run(test.Name("bf", tc.branchFactor, "size", tc.size), func(t *testing.T) {
			// Use node 2 (first child of root, has its own children)
			intermediateID := hotstuff.ID(2)
			setup := wireUpKauri(t, intermediateID, tc.size, tc.branchFactor)
			simulateConnection(t, setup)

			proposal := createProposal(t, setup.essentials, 1)
			setup.essentials.Blockchain().Store(proposal.Block)
			pc := testutil.CreatePC(t, proposal.Block, setup.essentials.Authority())

			// Verify this node has children (precondition)
			children := setup.tree.ReplicaChildren()
			if len(children) == 0 {
				t.Error("intermediate node should have children")
			}

			err := setup.kauri.Disseminate(proposal, pc)
			if err != nil {
				t.Fatalf("Disseminate failed: %v", err)
			}

			// No contribution yet (waiting for children)
			contributions := setup.sender.ContributionsSent()
			if len(contributions) != 0 {
				t.Errorf("expected no contributions immediately, got %d", len(contributions))
			}

			// Wait for WaitTimerExpiredEvent to be processed
			waitForEvent[comm.WaitTimerExpiredEvent](t, setup.essentials.EventLoop(), 100*time.Millisecond)

			// Now contribution should be sent (timer expired)
			contributions = setup.sender.ContributionsSent()
			if len(contributions) != 1 {
				t.Errorf("expected 1 contribution after timer expiry, got %d", len(contributions))
			}
		})
	}
}

// TestDisseminateFromRoot tests that the root node with children
// starts the aggregation timer and eventually sends its contribution to parent.
func TestDisseminateFromRoot(t *testing.T) {
	for _, tc := range standardTreeConfigs() {
		t.Run(test.Name("bf", tc.branchFactor, "size", tc.size), func(t *testing.T) {
			rootID := hotstuff.ID(1)
			setup := wireUpKauri(t, rootID, tc.size, tc.branchFactor)
			simulateConnection(t, setup)

			proposal := createProposal(t, setup.essentials, rootID)
			setup.essentials.Blockchain().Store(proposal.Block)
			pc := testutil.CreatePC(t, proposal.Block, setup.essentials.Authority())

			// Verify root has children (precondition for multi-node trees)
			children := setup.tree.ReplicaChildren()
			if len(children) == 0 {
				t.Error("root should have children in multi-node tree")
			}

			err := setup.kauri.Disseminate(proposal, pc)
			if err != nil {
				t.Fatalf("Disseminate failed: %v", err)
			}

			// No contribution yet (waiting for children)
			contributions := setup.sender.ContributionsSent()
			if len(contributions) != 0 {
				t.Errorf("expected no contributions immediately, got %d", len(contributions))
			}

			// Wait for WaitTimerExpiredEvent to be processed
			waitForEvent[comm.WaitTimerExpiredEvent](t, setup.essentials.EventLoop(), 100*time.Millisecond)

			// Now contribution should be sent (timer expired)
			contributions = setup.sender.ContributionsSent()
			if len(contributions) != 1 {
				t.Errorf("expected 1 contribution after timer expiry, got %d", len(contributions))
			}
		})
	}
}

// TestAggregateFromLeaf tests that a leaf node sends its vote contribution
// to its parent.
func TestAggregateFromLeaf(t *testing.T) {
	for _, tc := range standardTreeConfigs() {
		t.Run(test.Name("bf", tc.branchFactor, "size", tc.size), func(t *testing.T) {
			leafID := hotstuff.ID(tc.size)
			setup := wireUpKauri(t, leafID, tc.size, tc.branchFactor)
			simulateConnection(t, setup)

			proposal := createProposal(t, setup.essentials, 1)
			setup.essentials.Blockchain().Store(proposal.Block)
			pc := testutil.CreatePC(t, proposal.Block, setup.essentials.Authority())

			err := setup.kauri.Aggregate(proposal, pc)
			if err != nil {
				t.Fatalf("Aggregate failed: %v", err)
			}

			// Leaf should send contribution to parent
			contributions := setup.sender.ContributionsSent()
			if len(contributions) != 1 {
				t.Errorf("expected 1 contribution from leaf, got %d", len(contributions))
			}

			// Verify the contribution has the correct view
			if contributions[0].View != proposal.Block.View() {
				t.Errorf("contribution view = %d, want %d", contributions[0].View, proposal.Block.View())
			}
		})
	}
}

// TestDisseminateDelayedUntilConnected tests that Disseminate waits for
// connection before processing.
func TestDisseminateDelayedUntilConnected(t *testing.T) {
	for _, tc := range standardTreeConfigs() {
		t.Run(test.Name("bf", tc.branchFactor, "size", tc.size), func(t *testing.T) {
			leafID := hotstuff.ID(tc.size)
			setup := wireUpKauri(t, leafID, tc.size, tc.branchFactor)

			// Do NOT simulate connection yet
			proposal := createProposal(t, setup.essentials, 1)
			setup.essentials.Blockchain().Store(proposal.Block)
			pc := testutil.CreatePC(t, proposal.Block, setup.essentials.Authority())

			err := setup.kauri.Disseminate(proposal, pc)
			if err != nil {
				t.Fatalf("Disseminate failed: %v", err)
			}

			// Nothing should be sent yet
			contributions := setup.sender.ContributionsSent()
			if len(contributions) != 0 {
				t.Errorf("expected no contributions before connection, got %d", len(contributions))
			}

			// Simulate connection: ReplicaConnectedEvent first (sets initDone=true),
			// then ConnectedEvent (triggers the delayed WaitForConnectedEvent)
			el := setup.essentials.EventLoop()
			el.AddEvent(hotstuff.ReplicaConnectedEvent{})
			el.Tick(context.Background()) // Process ReplicaConnectedEvent

			el.AddEvent(network.ConnectedEvent{})
			// Wait for WaitForConnectedEvent to be processed
			waitForEvent[comm.WaitForConnectedEvent](t, el, 100*time.Millisecond)

			// Now contribution should be sent
			contributions = setup.sender.ContributionsSent()
			if len(contributions) != 1 {
				t.Errorf("expected 1 contribution after connection, got %d", len(contributions))
			}
		})
	}
}

// TestAggregateDelayedUntilConnected tests that Aggregate waits for
// connection before processing.
func TestAggregateDelayedUntilConnected(t *testing.T) {
	for _, tc := range standardTreeConfigs() {
		t.Run(test.Name("bf", tc.branchFactor, "size", tc.size), func(t *testing.T) {
			leafID := hotstuff.ID(tc.size)
			setup := wireUpKauri(t, leafID, tc.size, tc.branchFactor)

			// Do NOT simulate connection yet
			proposal := createProposal(t, setup.essentials, 1)
			setup.essentials.Blockchain().Store(proposal.Block)
			pc := testutil.CreatePC(t, proposal.Block, setup.essentials.Authority())

			err := setup.kauri.Aggregate(proposal, pc)
			if err != nil {
				t.Fatalf("Aggregate failed: %v", err)
			}

			// Nothing should be sent yet
			contributions := setup.sender.ContributionsSent()
			if len(contributions) != 0 {
				t.Errorf("expected no contributions before connection, got %d", len(contributions))
			}

			// Simulate connection: ReplicaConnectedEvent first (sets initDone=true),
			// then ConnectedEvent (triggers the delayed WaitForConnectedEvent)
			el := setup.essentials.EventLoop()
			el.AddEvent(hotstuff.ReplicaConnectedEvent{})
			el.Tick(context.Background()) // Process ReplicaConnectedEvent

			el.AddEvent(network.ConnectedEvent{})
			// Wait for WaitForConnectedEvent to be processed
			waitForEvent[comm.WaitForConnectedEvent](t, el, 100*time.Millisecond)

			// Now contribution should be sent
			contributions = setup.sender.ContributionsSent()
			if len(contributions) != 1 {
				t.Errorf("expected 1 contribution after connection, got %d", len(contributions))
			}
		})
	}
}

// TestSingleNodeTree tests behavior when there is only one node in the tree.
func TestSingleNodeTree(t *testing.T) {
	setup := wireUpKauri(t, 1, 1, 2)
	simulateConnection(t, setup)

	proposal := createProposal(t, setup.essentials, 1)
	setup.essentials.Blockchain().Store(proposal.Block)
	pc := testutil.CreatePC(t, proposal.Block, setup.essentials.Authority())

	err := setup.kauri.Disseminate(proposal, pc)
	if err != nil {
		t.Fatalf("Disseminate failed: %v", err)
	}

	// Single node has no children, should send contribution immediately
	contributions := setup.sender.ContributionsSent()
	if len(contributions) != 1 {
		t.Errorf("expected 1 contribution from single node, got %d", len(contributions))
	}
}

// TestChildrenCount tests that the correct number of children receive proposals
// for different tree configurations.
func TestChildrenCount(t *testing.T) {
	tests := []struct {
		branchFactor int
		size         int
		replicaID    hotstuff.ID
		wantChildren int
	}{
		// Root nodes
		{branchFactor: 2, size: 7, replicaID: 1, wantChildren: 2},
		{branchFactor: 3, size: 13, replicaID: 1, wantChildren: 3},
		{branchFactor: 4, size: 21, replicaID: 1, wantChildren: 4},
		{branchFactor: 5, size: 31, replicaID: 1, wantChildren: 5},
		// Intermediate nodes (node 2)
		{branchFactor: 2, size: 7, replicaID: 2, wantChildren: 2},
		{branchFactor: 3, size: 13, replicaID: 2, wantChildren: 3},
		{branchFactor: 4, size: 21, replicaID: 2, wantChildren: 4},
		// Leaf nodes (last node)
		{branchFactor: 2, size: 7, replicaID: 7, wantChildren: 0},
		{branchFactor: 3, size: 13, replicaID: 13, wantChildren: 0},
		{branchFactor: 4, size: 21, replicaID: 21, wantChildren: 0},
	}

	for _, tt := range tests {
		t.Run(test.Name("bf", tt.branchFactor, "id", int(tt.replicaID)), func(t *testing.T) {
			setup := wireUpKauri(t, tt.replicaID, tt.size, tt.branchFactor)
			children := setup.tree.ReplicaChildren()
			if len(children) != tt.wantChildren {
				t.Errorf("ReplicaChildren() = %d, want %d", len(children), tt.wantChildren)
			}
		})
	}
}

// TestTreeNodePositions verifies that tree positions work correctly
// for different node roles.
func TestTreeNodePositions(t *testing.T) {
	tests := []struct {
		branchFactor int
		size         int
		replicaID    hotstuff.ID
		wantIsRoot   bool
		wantIsLeaf   bool
	}{
		// bf=2, size=7
		{branchFactor: 2, size: 7, replicaID: 1, wantIsRoot: true, wantIsLeaf: false},
		{branchFactor: 2, size: 7, replicaID: 2, wantIsRoot: false, wantIsLeaf: false},
		{branchFactor: 2, size: 7, replicaID: 7, wantIsRoot: false, wantIsLeaf: true},
		// bf=4, size=21
		{branchFactor: 4, size: 21, replicaID: 1, wantIsRoot: true, wantIsLeaf: false},
		{branchFactor: 4, size: 21, replicaID: 5, wantIsRoot: false, wantIsLeaf: false},
		{branchFactor: 4, size: 21, replicaID: 21, wantIsRoot: false, wantIsLeaf: true},
	}

	for _, tt := range tests {
		t.Run(test.Name("bf", tt.branchFactor, "id", int(tt.replicaID)), func(t *testing.T) {
			setup := wireUpKauri(t, tt.replicaID, tt.size, tt.branchFactor)

			isRoot := setup.tree.IsRoot(tt.replicaID)
			if isRoot != tt.wantIsRoot {
				t.Errorf("IsRoot(%d) = %v, want %v", tt.replicaID, isRoot, tt.wantIsRoot)
			}

			// A node is a leaf if it has no children
			isLeaf := len(setup.tree.ReplicaChildren()) == 0
			if isLeaf != tt.wantIsLeaf {
				t.Errorf("IsLeaf(%d) = %v, want %v", tt.replicaID, isLeaf, tt.wantIsLeaf)
			}
		})
	}
}

// TestProposalViewTracking ensures that the view is correctly tracked
// across dissemination and aggregation.
func TestProposalViewTracking(t *testing.T) {
	views := []hotstuff.View{1, 5, 10, 100}

	for _, view := range views {
		t.Run(test.Name("view", int(view)), func(t *testing.T) {
			leafID := hotstuff.ID(7) // leaf in bf=2, size=7 tree
			setup := wireUpKauri(t, leafID, 7, 2)
			simulateConnection(t, setup)

			// Create block with specific view
			qc := testutil.CreateQC(t, hotstuff.GetGenesis(), setup.essentials.Authority())
			block := hotstuff.NewBlock(
				hotstuff.GetGenesis().Hash(),
				qc,
				&clientpb.Batch{},
				view,
				1,
			)
			setup.essentials.Blockchain().Store(block)

			proposal := &hotstuff.ProposeMsg{ID: 1, Block: block}
			pc := testutil.CreatePC(t, block, setup.essentials.Authority())

			err := setup.kauri.Disseminate(proposal, pc)
			if err != nil {
				t.Fatalf("Disseminate failed: %v", err)
			}

			contributions := setup.sender.ContributionsSent()
			if len(contributions) != 1 {
				t.Fatalf("expected 1 contribution, got %d", len(contributions))
			}

			if contributions[0].View != view {
				t.Errorf("contribution view = %d, want %d", contributions[0].View, view)
			}
		})
	}
}
