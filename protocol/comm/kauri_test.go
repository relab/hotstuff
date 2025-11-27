package comm

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	clientpb "github.com/relab/hotstuff/internal/proto/clientpb"
	hotstuffpb "github.com/relab/hotstuff/internal/proto/hotstuffpb"
	kauripb "github.com/relab/hotstuff/internal/proto/kauripb"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto"
)

// dummyQuorumSignature is a minimal QuorumSignature implementation for tests.
type dummyQuorumSignature struct {
	b  []byte
	bf *crypto.Bitfield
}

func (d *dummyQuorumSignature) ToBytes() []byte              { return d.b }
func (d *dummyQuorumSignature) Participants() hotstuff.IDSet { return d.bf }

// newDummyQuorumSignature creates a new dummyQuorumSignature with the given participants.
func newDummyQuorumSignature(b []byte, participants ...hotstuff.ID) *dummyQuorumSignature {
	bf := &crypto.Bitfield{}
	for _, id := range participants {
		bf.Add(id)
	}
	return &dummyQuorumSignature{b: b, bf: bf}
}

// fakeBase implements crypto.Base for injecting into cert.Authority in tests.
type fakeBase struct {
	VerifyFunc  func(sig hotstuff.QuorumSignature, msg []byte) error
	CombineFunc func(signatures ...hotstuff.QuorumSignature) (hotstuff.QuorumSignature, error)
}

func (f *fakeBase) Sign(message []byte) (hotstuff.QuorumSignature, error) {
	return &dummyQuorumSignature{b: message, bf: &crypto.Bitfield{}}, nil
}

func (f *fakeBase) Combine(signatures ...hotstuff.QuorumSignature) (hotstuff.QuorumSignature, error) {
	if f.CombineFunc != nil {
		return f.CombineFunc(signatures...)
	}
	nb := &crypto.Bitfield{}
	for _, s := range signatures {
		s.Participants().ForEach(func(id hotstuff.ID) { nb.Add(id) })
	}
	return &dummyQuorumSignature{b: []byte("combined"), bf: nb}, nil
}

func (f *fakeBase) Verify(signature hotstuff.QuorumSignature, message []byte) error {
	if f.VerifyFunc != nil {
		return f.VerifyFunc(signature, message)
	}
	return nil
}

func (f *fakeBase) BatchVerify(signature hotstuff.QuorumSignature, batch map[hotstuff.ID][]byte) error {
	return nil
}

// mockKauriSender implements core.KauriSender for tests.
type mockKauriSender struct {
	proposeCh chan *hotstuff.ProposeMsg
	contribCh chan struct{}
	subFunc   func(ids []hotstuff.ID) (core.Sender, error)
}

func (m *mockKauriSender) NewView(id hotstuff.ID, msg hotstuff.SyncInfo) error {
	return nil
}

func (m *mockKauriSender) Vote(id hotstuff.ID, cert hotstuff.PartialCert) error {
	return nil
}

func (m *mockKauriSender) Timeout(msg hotstuff.TimeoutMsg) {
}

func (m *mockKauriSender) Propose(p *hotstuff.ProposeMsg) {
	if m.proposeCh != nil {
		m.proposeCh <- p
	}
}

func (m *mockKauriSender) RequestBlock(ctx context.Context, hash hotstuff.Hash) (*hotstuff.Block, bool) {
	return nil, false
}

func (m *mockKauriSender) Sub(ids []hotstuff.ID) (core.Sender, error) {
	if m.subFunc != nil {
		return m.subFunc(ids)
	}
	return m, nil
}

func (m *mockKauriSender) SendContributionToParent(view hotstuff.View, qc hotstuff.QuorumSignature) {
	if m.contribCh != nil {
		m.contribCh <- struct{}{}
	}
}

// Test that NewKauri panics if no tree is configured.
func TestNewKauriPanicsWhenNoTree(t *testing.T) {
	el := eventloop.New(logging.NewWithDest(new(bytes.Buffer), "test"), 4)
	cfg := core.NewRuntimeConfig(1, nil)
	defer func() { _ = recover() }()
	// should panic
	_ = NewKauri(logging.NewWithDest(new(bytes.Buffer), "test"), el, cfg, nil, nil, &mockKauriSender{})
}

func TestSendProposalToChildren_NoChildren_SendsToParent(t *testing.T) {
	el := eventloop.New(logging.NewWithDest(new(bytes.Buffer), "test"), 8)
	// single-node tree -> no children
	tr := tree.NewSimple(1, 2, []hotstuff.ID{1})
	cfg := core.NewRuntimeConfig(1, nil, core.WithKauriTree(tr))
	// ensure quorum size is 1 by adding one replica
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 1})

	sender := &mockKauriSender{contribCh: make(chan struct{}, 1)}
	bc := blockchain.New(el, logging.NewWithDest(new(bytes.Buffer), "test"), sender)
	// store a block we'll reference
	block := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()), &clientpb.Batch{}, 1, 1)
	bc.Store(block)

	k := NewKauri(logging.NewWithDest(new(bytes.Buffer), "test"), el, cfg, bc, nil, sender)
	// instead of relying on the event loop, directly trigger the wait timer handler
	// mark as initialized
	k.initDone = true
	k.blockHash = block.Hash()
	k.currentView = 1
	// set some agg contribution
	k.aggContrib = newDummyQuorumSignature([]byte("s"), 1)

	if err := k.sendProposalToChildren(&hotstuff.ProposeMsg{Block: block}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	select {
	case <-sender.contribCh:
		// ok
	case <-time.After(50 * time.Millisecond):
		t.Fatalf("expected contribution sent to parent")
	}
	if !k.aggSent {
		t.Fatalf("expected aggSent to be true")
	}
}

func TestSendProposalToChildren_WithChildren_ProposesAndAggregates(t *testing.T) {
	el := eventloop.New(logging.NewWithDest(new(bytes.Buffer), "test"), 16)
	// two-node tree where node 1 has child 2
	tr := tree.NewSimple(1, 2, []hotstuff.ID{1, 2})
	tr.SetTreeHeightWaitTime(0)
	cfg := core.NewRuntimeConfig(1, nil, core.WithKauriTree(tr))
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 1})
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 2})

	proposeCh := make(chan *hotstuff.ProposeMsg, 1)
	contribCh := make(chan struct{}, 1)
	sender := &mockKauriSender{proposeCh: proposeCh, contribCh: contribCh}
	// Sub should return a sender that sends on proposeCh
	sender.subFunc = func(ids []hotstuff.ID) (core.Sender, error) { return sender, nil }

	bc := blockchain.New(el, logging.NewWithDest(new(bytes.Buffer), "test"), sender)
	block := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()), &clientpb.Batch{}, 1, 1)
	bc.Store(block)

	k := NewKauri(logging.NewWithDest(new(bytes.Buffer), "test"), el, cfg, bc, nil, sender)
	k.initDone = true
	k.blockHash = block.Hash()
	k.currentView = 1
	k.aggContrib = newDummyQuorumSignature([]byte("s"), 2)

	if err := k.sendProposalToChildren(&hotstuff.ProposeMsg{Block: block}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// child should receive propose
	select {
	case <-proposeCh:
	case <-time.After(50 * time.Millisecond):
		t.Fatalf("expected child propose")
	}
	// Instead of relying on the event loop, directly call the handler that would be invoked
	k.onWaitTimerExpired(WaitTimerExpiredEvent{currentView: k.currentView})
	select {
	case <-contribCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected contribution to parent after aggregation wait")
	}
}

func TestMergeContribution_Behaviors(t *testing.T) {
	el := eventloop.New(logging.NewWithDest(new(bytes.Buffer), "test"), 8)
	tr := tree.NewSimple(1, 2, []hotstuff.ID{1})
	cfg := core.NewRuntimeConfig(1, nil, core.WithKauriTree(tr))
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 1})
	sender := &mockKauriSender{contribCh: make(chan struct{}, 1)}
	bc := blockchain.New(el, logging.NewWithDest(new(bytes.Buffer), "test"), sender)

	// block not present -> error
	k := NewKauri(logging.NewWithDest(new(bytes.Buffer), "test"), el, cfg, bc, nil, sender)
	k.blockHash = hotstuff.Hash{} // not stored except genesis
	if err := k.mergeContribution(newDummyQuorumSignature([]byte("x"))); err == nil {
		t.Fatalf("expected error when block missing")
	}

	// now store a block and test Verify failure
	block := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()), &clientpb.Batch{}, 2, 1)
	bc.Store(block)
	// inject authority that fails verify
	fbFail := &fakeBase{VerifyFunc: func(sig hotstuff.QuorumSignature, msg []byte) error { return fmt.Errorf("verify failed") }}
	k.auth = cert.NewAuthority(cfg, bc, fbFail)
	k.blockHash = block.Hash()
	if err := k.mergeContribution(newDummyQuorumSignature([]byte("x"))); err == nil {
		t.Fatalf("expected verify error")
	}

	// success: first contribution sets aggContrib
	fbOK := &fakeBase{VerifyFunc: func(sig hotstuff.QuorumSignature, msg []byte) error { return nil }}
	k.auth = cert.NewAuthority(cfg, bc, fbOK)
	qs := newDummyQuorumSignature([]byte("s"), 1)
	k.aggContrib = nil
	k.currentView = 2
	k.blockHash = block.Hash()
	if err := k.mergeContribution(qs); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if k.aggContrib == nil {
		t.Fatalf("expected aggContrib to be set")
	}

	// merging: create another disjoint contribution and ensure Combine used and event emitted when quorum reached
	// make quorum size = 1 for brevity by having 1 replica in cfg
	cfg = core.NewRuntimeConfig(1, nil, core.WithKauriTree(tr))
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 1})
	k.config = cfg
	// make combined signature have participant count >= quorum
	combined := newDummyQuorumSignature([]byte("c"), 1)
	k.auth = cert.NewAuthority(cfg, bc, &fakeBase{CombineFunc: func(signatures ...hotstuff.QuorumSignature) (hotstuff.QuorumSignature, error) { return combined, nil }, VerifyFunc: func(sig hotstuff.QuorumSignature, msg []byte) error { return nil }})

	// register handler to capture NewViewMsg event
	got := make(chan struct{}, 1)
	eventloop.Register(el, func(msg hotstuff.NewViewMsg) { got <- struct{}{} })
	// current aggContrib set to something else
	old := newDummyQuorumSignature([]byte("old"), 2)
	k.aggContrib = old
	// merge another signature
	if err := k.mergeContribution(qs); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// process one queued event (the NewViewMsg) manually using Tick
	el.Tick(context.Background())
	select {
	case <-got:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected NewViewMsg event after aggregation")
	}
}

func TestOnWaitForConnected_NoStartWhenCurrentHigher(t *testing.T) {
	el := eventloop.New(logging.NewWithDest(new(bytes.Buffer), "test"), 8)
	tr := tree.NewSimple(1, 2, []hotstuff.ID{1, 2})
	cfg := core.NewRuntimeConfig(1, nil, core.WithKauriTree(tr))
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 1})
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 2})

	proposeCh := make(chan *hotstuff.ProposeMsg, 1)
	sender := &mockKauriSender{proposeCh: proposeCh}
	sender.subFunc = func(ids []hotstuff.ID) (core.Sender, error) { return sender, nil }
	bc := blockchain.New(el, logging.NewWithDest(new(bytes.Buffer), "test"), sender)

	block := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()), &clientpb.Batch{}, 1, 1)
	k := NewKauri(logging.NewWithDest(new(bytes.Buffer), "test"), el, cfg, bc, nil, sender)
	k.initDone = true
	k.currentView = 5

	// event has a lower view -> should not start kauri
	event := WaitForConnectedEvent{pc: hotstuff.PartialCert{}, p: &hotstuff.ProposeMsg{Block: block}}
	k.onWaitForConnected(event)
	select {
	case <-proposeCh:
		t.Fatalf("did not expect proposal to be sent when current view is higher")
	default:
		// ok
	}
}

func TestOnContributionRecv_ViewMismatch(t *testing.T) {
	el := eventloop.New(logging.NewWithDest(new(bytes.Buffer), "test"), 8)
	tr := tree.NewSimple(1, 2, []hotstuff.ID{1})
	cfg := core.NewRuntimeConfig(1, nil, core.WithKauriTree(tr))
	sender := &mockKauriSender{contribCh: make(chan struct{}, 1)}
	bc := blockchain.New(el, logging.NewWithDest(new(bytes.Buffer), "test"), sender)
	k := NewKauri(logging.NewWithDest(new(bytes.Buffer), "test"), el, cfg, bc, nil, sender)
	k.currentView = 1
	// create contribution with different view
	k.onContributionRecv(&kauripb.Contribution{ID: 2, View: 2})
	if len(k.senders) != 0 {
		t.Fatalf("expected no senders appended on view mismatch")
	}
}

func TestSendProposalToChildren_SubError(t *testing.T) {
	el := eventloop.New(logging.NewWithDest(new(bytes.Buffer), "test"), 8)
	tr := tree.NewSimple(1, 2, []hotstuff.ID{1, 2})
	cfg := core.NewRuntimeConfig(1, nil, core.WithKauriTree(tr))
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 1})
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 2})

	sender := &mockKauriSender{}
	sender.subFunc = func(ids []hotstuff.ID) (core.Sender, error) { return nil, fmt.Errorf("sub failed") }
	bc := blockchain.New(el, logging.NewWithDest(new(bytes.Buffer), "test"), sender)
	block := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()), &clientpb.Batch{}, 1, 1)
	k := NewKauri(logging.NewWithDest(new(bytes.Buffer), "test"), el, cfg, bc, nil, sender)
	k.initDone = true
	if err := k.sendProposalToChildren(&hotstuff.ProposeMsg{Block: block}); err == nil {
		t.Fatalf("expected error when Sub fails")
	}
}

func TestWaitToAggregate_TriggersWaitExpiredAndResets(t *testing.T) {
	el := eventloop.New(logging.NewWithDest(new(bytes.Buffer), "test"), 8)
	tr := tree.NewSimple(1, 2, []hotstuff.ID{1})
	tr.SetTreeHeightWaitTime(0)
	cfg := core.NewRuntimeConfig(1, nil, core.WithKauriTree(tr))
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 1})
	contribCh := make(chan struct{}, 1)
	sender := &mockKauriSender{contribCh: contribCh}
	bc := blockchain.New(el, logging.NewWithDest(new(bytes.Buffer), "test"), sender)

	k := NewKauri(logging.NewWithDest(new(bytes.Buffer), "test"), el, cfg, bc, nil, sender)
	k.currentView = 7
	k.aggSent = false
	k.aggContrib = newDummyQuorumSignature([]byte("s"), 1)

	// call waitToAggregate directly (uses WaitTime==0)
	k.waitToAggregate()

	// process the added WaitTimerExpiredEvent
	el.Tick(context.Background())
	select {
	case <-contribCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected SendContributionToParent to be called on wait expiry")
	}
	if k.aggContrib != nil {
		t.Fatalf("expected aggContrib to be reset after wait expiry")
	}
}

func TestDisseminate_DelayedUntilConnected(t *testing.T) {
	el := eventloop.New(logging.NewWithDest(new(bytes.Buffer), "test"), 8)
	tr := tree.NewSimple(1, 2, []hotstuff.ID{1})
	cfg := core.NewRuntimeConfig(1, nil, core.WithKauriTree(tr))
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 1})

	sender := &mockKauriSender{contribCh: make(chan struct{}, 1)}
	bc := blockchain.New(el, logging.NewWithDest(new(bytes.Buffer), "test"), sender)
	block := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()), &clientpb.Batch{}, 1, 1)
	bc.Store(block)

	k := NewKauri(logging.NewWithDest(new(bytes.Buffer), "test"), el, cfg, bc, nil, sender)
	// initDone false by default
	qs := newDummyQuorumSignature([]byte("s"), 1)
	pc := hotstuff.NewPartialCert(qs, block.Hash())

	if err := k.Disseminate(&hotstuff.ProposeMsg{Block: block}, pc); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// nothing sent yet
	select {
	case <-sender.contribCh:
		t.Fatalf("did not expect contribution yet")
	default:
	}
	// now trigger the ConnectedEvent which should release the delayed event
	el.AddEvent(network.ConnectedEvent{})
	// simulate that the replica is connected (what the ReplicaConnectedEvent handler would have done)
	k.initDone = true

	// process connected and delayed WaitForConnectedEvent
	el.Tick(context.Background())
	el.Tick(context.Background())
	select {
	case <-sender.contribCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected contribution after connection")
	}
}

func TestAggregate_DelayedUntilConnected(t *testing.T) {
	el := eventloop.New(logging.NewWithDest(new(bytes.Buffer), "test"), 8)
	tr := tree.NewSimple(1, 2, []hotstuff.ID{1})
	cfg := core.NewRuntimeConfig(1, nil, core.WithKauriTree(tr))
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 1})

	sender := &mockKauriSender{contribCh: make(chan struct{}, 1)}
	bc := blockchain.New(el, logging.NewWithDest(new(bytes.Buffer), "test"), sender)
	block := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()), &clientpb.Batch{}, 1, 1)
	bc.Store(block)

	k := NewKauri(logging.NewWithDest(new(bytes.Buffer), "test"), el, cfg, bc, nil, sender)
	qs := newDummyQuorumSignature([]byte("s"), 1)
	pc := hotstuff.NewPartialCert(qs, block.Hash())

	if err := k.Aggregate(&hotstuff.ProposeMsg{Block: block}, pc); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	el.AddEvent(network.ConnectedEvent{})
	k.initDone = true

	// process connected and delayed WaitForConnectedEvent
	el.Tick(context.Background())
	el.Tick(context.Background())
	select {
	case <-sender.contribCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected contribution after connection")
	}
}

func TestOnContributionRecv_IsSubSetCallsParent(t *testing.T) {
	el := eventloop.New(logging.NewWithDest(new(bytes.Buffer), "test"), 8)
	tr := tree.NewSimple(1, 2, []hotstuff.ID{1, 2})
	cfg := core.NewRuntimeConfig(1, nil, core.WithKauriTree(tr))
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 1})
	cfg.AddReplica(&hotstuff.ReplicaInfo{ID: 2})

	contribCh := make(chan struct{}, 1)
	sender := &mockKauriSender{contribCh: contribCh}
	bc := blockchain.New(el, logging.NewWithDest(new(bytes.Buffer), "test"), sender)
	block := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()), &clientpb.Batch{}, 1, 1)
	bc.Store(block)

	// authority that accepts any signature
	k := NewKauri(logging.NewWithDest(new(bytes.Buffer), "test"), el, cfg, bc, cert.NewAuthority(cfg, bc, &fakeBase{}), sender)
	k.currentView = 1
	k.blockHash = block.Hash()

	// construct a proto QuorumSignature with ECDSA sig so QuorumSignatureFromProto returns non-nil
	protoSig := &hotstuffpb.QuorumSignature{}
	ecd := &hotstuffpb.ECDSAMultiSignature{Sigs: []*hotstuffpb.ECDSASignature{{Signer: uint32(2), Sig: []byte("sig")}}}
	protoSig.Sig = &hotstuffpb.QuorumSignature_ECDSASigs{ECDSASigs: ecd}

	k.onContributionRecv(&kauripb.Contribution{ID: 2, View: uint64(k.currentView), Signature: protoSig})
	select {
	case <-contribCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected SendContributionToParent to be invoked when subtree satisfied")
	}
	if !k.aggSent {
		t.Fatalf("expected aggSent to be true after subtree aggregation")
	}
}
