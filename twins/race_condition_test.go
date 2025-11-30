package twins_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/crypto"
	"github.com/relab/hotstuff/wiring"
)

// TestFetchBlockIsBlocking verifies that FetchBlock (via blockchain.Get)
// is a blocking/synchronous operation. This is critical because if FetchBlock
// were async, the subsequent Verify would fail immediately and the message
// would be dropped.
//
// Current implementation: blockchain.Get() blocks until RequestBlock returns.
// This test validates that behavior.
func TestFetchBlockIsBlocking(t *testing.T) {
	// Create a delayed sender that introduces artificial latency
	set := testutil.NewEssentialsSet(t, 4, crypto.NameECDSA)
	subject := set[0]

	// Create a block that will need to be fetched
	blockToFetch := testutil.CreateBlock(t, subject.Authority())

	// Store the block in other nodes' blockchains, but NOT in subject's
	for i := 1; i < len(set); i++ {
		set[i].Blockchain().Store(blockToFetch)
	}

	// Create a delayed mock sender
	fetchDelay := 100 * time.Millisecond
	delayedSender := &delayedMockSender{
		MockSender: testutil.NewMockSender(subject.RuntimeCfg().ID()),
		fetchDelay: fetchDelay,
	}
	// Add other blockchains as sources
	for i := 1; i < len(set); i++ {
		delayedSender.AddBlockchain(set[i].Blockchain())
	}

	// Create blockchain with delayed sender
	bc := blockchain.New(
		subject.EventLoop(),
		subject.Logger(),
		delayedSender,
	)

	// Verify block is NOT in local storage
	_, ok := bc.LocalGet(blockToFetch.Hash())
	if ok {
		t.Fatal("Block should not be in local storage initially")
	}

	// Time the Get operation - it should block for at least fetchDelay
	start := time.Now()
	block, ok := bc.Get(blockToFetch.Hash())
	elapsed := time.Since(start)

	if !ok {
		t.Fatal("Get should have succeeded after fetching")
	}
	if block.Hash() != blockToFetch.Hash() {
		t.Fatal("Got wrong block")
	}

	// Verify that Get blocked for the expected delay
	if elapsed < fetchDelay {
		t.Errorf("Get returned too fast (%v), expected at least %v. "+
			"This suggests Get is async, which would cause race conditions!",
			elapsed, fetchDelay)
	}

	t.Logf("Get blocked for %v (delay: %v) - blocking behavior confirmed", elapsed, fetchDelay)
}

// TestAsyncFetchRaceCondition demonstrates the race condition that would occur
// if FetchBlock were async while Verify is synchronous.
//
// This test simulates the problematic scenario:
// 1. Node receives Proposal with QC referencing unknown Block A
// 2. FetchBlock(A) is initiated (with delay)
// 3. Verify is called immediately
// 4. If Verify runs before Fetch completes, it fails
//
// Expected: With current sync implementation, this should pass.
// If someone changes Get() to be async, this test will fail.
func TestAsyncFetchRaceCondition(t *testing.T) {
	const fetchDelay = 200 * time.Millisecond

	// Setup 4-node network
	set := testutil.NewEssentialsSet(t, 4, crypto.NameECDSA)
	subject := set[0]

	// Create blocks: genesis -> blockA -> blockB (proposal)
	blockA := testutil.CreateBlock(t, subject.Authority())
	qcA := testutil.CreateQC(t, blockA, set.Signers()...)

	// Store blockA in other nodes only (NOT in subject)
	for i := 1; i < len(set); i++ {
		set[i].Blockchain().Store(blockA)
	}

	// Create delayed sender for subject
	delayedSender := &delayedMockSender{
		MockSender: testutil.NewMockSender(subject.RuntimeCfg().ID()),
		fetchDelay: fetchDelay,
	}
	for i := 1; i < len(set); i++ {
		delayedSender.AddBlockchain(set[i].Blockchain())
	}

	// Wire up subject with delayed sender
	base, _ := crypto.New(subject.RuntimeCfg(), crypto.NameECDSA)
	depsSecurity := wiring.NewSecurity(
		subject.EventLoop(),
		subject.Logger(),
		subject.RuntimeCfg(),
		delayedSender,
		base,
	)

	viewStates, err := protocol.NewViewStates(depsSecurity.Blockchain(), depsSecurity.Authority())
	if err != nil {
		t.Fatal(err)
	}

	// Verify blockA is NOT locally available
	_, ok := depsSecurity.Blockchain().LocalGet(blockA.Hash())
	if ok {
		t.Fatal("Block A should not be locally available")
	}

	// Track if FetchBlock was called and when it completed
	var fetchStarted, fetchCompleted atomic.Bool
	delayedSender.onFetchStart = func() { fetchStarted.Store(true) }
	delayedSender.onFetchComplete = func() { fetchCompleted.Store(true) }

	// Now simulate what advanceView does:
	// 1. FetchBlock (should block)
	// 2. Verify

	syncInfo := hotstuff.NewSyncInfoWith(qcA)

	// This should block until fetch completes
	viewStates.FetchBlock(qcA.BlockHash())

	// After FetchBlock returns, block should be available
	if !fetchCompleted.Load() {
		t.Error("Fetch did not complete - Get() may be async!")
	}

	// Now LocalGet should succeed
	_, ok = depsSecurity.Blockchain().LocalGet(blockA.Hash())
	if !ok {
		t.Error("RACE CONDITION DETECTED: Block not available after FetchBlock returned! " +
			"This means FetchBlock is async and Verify will fail.")
	}

	// Verify should now succeed
	if qc, ok := syncInfo.QC(); ok {
		err := depsSecurity.Authority().VerifyQuorumCert(qc)
		if err != nil {
			t.Errorf("RACE CONDITION: Verify failed after FetchBlock: %v", err)
		}
	}

	t.Log("No race condition detected - FetchBlock properly blocks until data is available")
}

// TestPendingMechanismRequired demonstrates why a Pending mechanism is needed
// if FetchBlock becomes async in the future.
//
// This test uses a mock async fetch to show the failure mode.
func TestPendingMechanismRequired(t *testing.T) {
	t.Skip("This test demonstrates what would happen if FetchBlock were async - " +
		"currently skipped because our implementation is sync")

	// This test would demonstrate:
	// 1. Async FetchBlock starts but doesn't block
	// 2. Verify runs immediately and fails (block not found)
	// 3. Message is dropped
	// 4. Later when block arrives, there's nothing to process
	//
	// The fix would be a PendingBuffer that:
	// - Stores messages waiting for blocks
	// - Retries when blocks arrive
}

// TestFullSynchronizerWithDelayedFetch tests the complete flow through Synchronizer
// with artificial network delay.
func TestFullSynchronizerWithDelayedFetch(t *testing.T) {
	const fetchDelay = 50 * time.Millisecond

	// Create essentials with custom delayed sender
	set := testutil.NewEssentialsSet(t, 4, crypto.NameECDSA)
	subject := set[0]

	// Create a chain of blocks
	block1 := testutil.CreateBlock(t, subject.Authority())

	// Store in all nodes except subject
	for i := 1; i < len(set); i++ {
		set[i].Blockchain().Store(block1)
	}

	// Create delayed sender
	delayedSender := &delayedMockSender{
		MockSender: testutil.NewMockSender(subject.RuntimeCfg().ID()),
		fetchDelay: fetchDelay,
	}
	for i := 1; i < len(set); i++ {
		delayedSender.AddBlockchain(set[i].Blockchain())
	}

	// Wire up full synchronizer
	base, _ := crypto.New(subject.RuntimeCfg(), crypto.NameECDSA)
	depsSecurity := wiring.NewSecurity(
		subject.EventLoop(),
		subject.Logger(),
		subject.RuntimeCfg(),
		delayedSender,
		base,
	)

	viewStates, _ := protocol.NewViewStates(depsSecurity.Blockchain(), depsSecurity.Authority())
	commandCache := clientpb.NewCommandCache(10)

	leaderRot := leaderrotation.NewFixed(1)
	consensusRules := rules.NewChainedHotStuff(
		subject.Logger(),
		subject.RuntimeCfg(),
		depsSecurity.Blockchain(),
	)

	votingMachine := votingmachine.New(
		subject.Logger(),
		subject.EventLoop(),
		subject.RuntimeCfg(),
		depsSecurity.Blockchain(),
		depsSecurity.Authority(),
		viewStates,
	)

	commAggregator := comm.NewClique(
		subject.RuntimeCfg(),
		votingMachine,
		leaderRot,
		delayedSender,
	)

	committer := consensus.NewCommitter(
		subject.EventLoop(),
		subject.Logger(),
		depsSecurity.Blockchain(),
		viewStates,
		consensusRules,
	)

	voter := consensus.NewVoter(
		subject.RuntimeCfg(),
		leaderRot,
		consensusRules,
		commAggregator,
		depsSecurity.Authority(),
		committer,
	)

	proposer := consensus.NewProposer(
		subject.EventLoop(),
		subject.RuntimeCfg(),
		depsSecurity.Blockchain(),
		viewStates,
		consensusRules,
		commAggregator,
		voter,
		commandCache,
		committer,
	)

	sync := synchronizer.New(
		subject.EventLoop(),
		subject.Logger(),
		subject.RuntimeCfg(),
		depsSecurity.Authority(),
		leaderRot,
		synchronizer.NewFixedDuration(time.Second),
		synchronizer.NewTimeoutRuler(subject.RuntimeCfg(), depsSecurity.Authority()),
		proposer,
		voter,
		viewStates,
		delayedSender,
	)

	_ = sync // sync is registered via event handlers

	// Create QC for block1
	qc := testutil.CreateQC(t, block1, set.Signers()...)

	// Initial view should be 1
	if viewStates.View() != 1 {
		t.Fatalf("Expected initial view 1, got %d", viewStates.View())
	}

	// Send NewViewMsg with QC referencing block1 (which subject doesn't have)
	newViewMsg := hotstuff.NewViewMsg{
		ID:          2,
		SyncInfo:    hotstuff.NewSyncInfoWith(qc),
		FromNetwork: true,
	}

	// Track timing
	start := time.Now()

	// Process the event
	subject.EventLoop().AddEvent(newViewMsg)

	// Run event loop for a bit
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	for i := 0; i < 100; i++ {
		subject.EventLoop().Tick(ctx)
		time.Sleep(5 * time.Millisecond)
	}

	elapsed := time.Since(start)

	// Check if view advanced
	finalView := viewStates.View()
	t.Logf("Processing took %v, view advanced from 1 to %d", elapsed, finalView)

	if finalView <= 1 {
		t.Errorf("VIEW DID NOT ADVANCE! This indicates a race condition or message was dropped. "+
			"View stayed at %d after receiving NewView with QC for view 1", finalView)
	}

	// Verify block1 is now in local storage
	_, ok := depsSecurity.Blockchain().LocalGet(block1.Hash())
	if !ok {
		t.Error("Block1 should be in local storage after processing")
	}
}

// delayedMockSender wraps MockSender and adds configurable delay to RequestBlock
type delayedMockSender struct {
	*testutil.MockSender
	fetchDelay      time.Duration
	onFetchStart    func()
	onFetchComplete func()
	mu              sync.Mutex
}

func (d *delayedMockSender) RequestBlock(ctx context.Context, hash hotstuff.Hash) (*hotstuff.Block, bool) {
	if d.onFetchStart != nil {
		d.onFetchStart()
	}

	// Introduce artificial delay
	select {
	case <-time.After(d.fetchDelay):
	case <-ctx.Done():
		return nil, false
	}

	// Delegate to underlying MockSender
	block, ok := d.MockSender.RequestBlock(ctx, hash)

	if d.onFetchComplete != nil {
		d.onFetchComplete()
	}

	return block, ok
}

// Ensure delayedMockSender implements core.Sender
var _ core.Sender = (*delayedMockSender)(nil)

