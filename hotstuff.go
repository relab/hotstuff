package hotstuff

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"sync"

	"github.com/relab/hotstuff/internal/logging"
)

var logger *log.Logger

func init() {
	logger = logging.GetLogger()
}

// Command is the client data that is processed by HotStuff
type Command string

// ReplicaID is the id of a replica
type ReplicaID uint32

// ReplicaInfo holds information about a replica
type ReplicaInfo struct {
	ID      ReplicaID
	Address string
	PubKey  *ecdsa.PublicKey
}

// ReplicaConfig holds information needed by a replica
type ReplicaConfig struct {
	Replicas   map[ReplicaID]*ReplicaInfo
	QuorumSize int
	BatchSize  int
}

// NewConfig returns a new ReplicaConfig instance
func NewConfig() *ReplicaConfig {
	return &ReplicaConfig{
		Replicas:  make(map[ReplicaID]*ReplicaInfo),
		BatchSize: 1,
	}
}

// Event is the type of notification sent to pacemaker
type Event uint8

// These are the types of events that can be sent to pacemaker
const (
	QCFinish Event = iota
	Propose
	ReceiveProposal
	HQCUpdate
	ReceiveNewView
)

// Notification sends information about a protocol state change to the pacemaker
type Notification struct {
	Event Event
	Block *Block
	QC    *QuorumCert
}

// Backend is the networking code that serves HotStuff
type Backend interface {
	Init(*HotStuff)
	Start() error
	DoPropose(context.Context, *Block) (*QuorumCert, error)
	DoNewView(context.Context, ReplicaID, *QuorumCert) error
	Close()
}

// HotStuff is the safety core of the HotStuff protocol
type HotStuff struct {
	Backend
	*ReplicaConfig

	id      ReplicaID
	mut     sync.Mutex
	privKey *ecdsa.PrivateKey

	// protocol data
	vHeight int
	genesis *Block
	bLock   *Block
	bExec   *Block
	bLeaf   *Block
	qcHigh  *QuorumCert
	blocks  BlockStorage

	SigCache *SignatureCache

	waitProposal chan struct{}

	// Notifications will be sent to pacemaker on this channel
	pmNotifyChan chan Notification

	// Contains the commands that are waiting to be proposed
	cmdCache *cmdSet

	pendingUpdates chan *Block

	// stops any goroutines started by HotStuff
	cancel context.CancelFunc

	Exec func([]Command)
}

// GetID returns the ID of this hotstuff instance
func (hs *HotStuff) GetID() ReplicaID {
	return hs.id
}

// GetHeight returns the height of the tree
func (hs *HotStuff) GetHeight() int {
	return hs.bLeaf.Height
}

// GetVotedHeight returns the height that was last voted at
func (hs *HotStuff) GetVotedHeight() int {
	return hs.vHeight
}

// GetLeaf returns the current leaf node of the tree
func (hs *HotStuff) GetLeaf() *Block {
	hs.mut.Lock()
	defer hs.mut.Unlock()
	return hs.bLeaf
}

// SetLeaf sets the leaf node of the tree
func (hs *HotStuff) SetLeaf(block *Block) {
	hs.mut.Lock()
	defer hs.mut.Unlock()
	hs.bLeaf = block
}

// GetQCHigh returns the highest valid Quorum Certificate known to the hotstuff instance.
func (hs *HotStuff) GetQCHigh() *QuorumCert {
	hs.mut.Lock()
	defer hs.mut.Unlock()
	return hs.qcHigh
}

// New creates a new Hotstuff instance
func New(id ReplicaID, privKey *ecdsa.PrivateKey, config *ReplicaConfig, backend Backend, exec func([]Command)) *HotStuff {
	logger.SetPrefix(fmt.Sprintf("hs(id %d): ", id))
	genesis := &Block{
		Committed: true,
	}
	qcForGenesis := CreateQuorumCert(genesis)
	blocks := NewMapStorage()
	blocks.Put(genesis)

	ctx, cancel := context.WithCancel(context.Background())

	hs := &HotStuff{
		Backend:        backend,
		ReplicaConfig:  config,
		id:             id,
		privKey:        privKey,
		genesis:        genesis,
		bLock:          genesis,
		bExec:          genesis,
		bLeaf:          genesis,
		qcHigh:         qcForGenesis,
		blocks:         blocks,
		SigCache:       NewSignatureCache(config),
		waitProposal:   make(chan struct{}),
		pmNotifyChan:   make(chan Notification, 10),
		cmdCache:       newCmdSet(),
		cancel:         cancel,
		pendingUpdates: make(chan *Block, 1),
		Exec:           exec,
	}

	go hs.updateAsync(ctx)

	backend.Init(hs)
	return hs
}

// Wait waits for a signal on the waitProposal channel.
func (hs *HotStuff) Wait() {
	select {
	case <-hs.waitProposal:
	}
}

// Signal wakes gorutines that has called Wait(). A signal is sent on the waitProposal channel. Signal does not block when called.
func (hs *HotStuff) Signal() {
	select {
	case hs.waitProposal <- struct{}{}:
	default:
	}
}

// AddCommand queues a command for proposal
func (hs *HotStuff) AddCommand(cmd Command) {
	hs.cmdCache.Add(cmd)
}

// GetNotifier returns a channel where the pacemaker can listen for notifications
func (hs *HotStuff) GetNotifier() <-chan Notification {
	return hs.pmNotifyChan
}

func (hs *HotStuff) pmNotify(n Notification) {
	hs.pmNotifyChan <- n
}

// This function will expect to return the block for the qc if it is delivered within a duration.
func (hs *HotStuff) expectBlockFor(qc *QuorumCert) (block *Block, ok bool) {

	block, ok = hs.blocks.BlockOf(qc)
	if ok {
		return block, ok
	}

	// Waits for a new proposal to be recived.
	hs.Wait()

	block, ok = hs.blocks.BlockOf(qc)

	if ok {
		return block, ok
	}

	logger.Println("ExpectBlockFor: Block not found")
	return
}

// UpdateQCHigh updates the qc held by the paceMaker, to the newest qc.
func (hs *HotStuff) UpdateQCHigh(qc *QuorumCert) bool {
	logger.Println("UpdateQCHigh")
	if !hs.SigCache.VerifyQuorumCert(qc) {
		logger.Println("QC not verified!: ", qc)
		return false
	}

	newQCHighBlock, ok := hs.expectBlockFor(qc)
	if !ok {
		logger.Println("Could not find block of new QC!")
		return false
	}

	hs.mut.Lock()
	defer hs.mut.Unlock()

	oldQCHighBlock, ok := hs.blocks.BlockOf(hs.qcHigh)
	if !ok {
		panic(fmt.Errorf("Block from the old qcHigh missing from storage"))
	}

	if newQCHighBlock.Height > oldQCHighBlock.Height {
		hs.qcHigh = qc
		hs.bLeaf = newQCHighBlock
		hs.pmNotify(Notification{HQCUpdate, newQCHighBlock, qc})
		return true
	}

	logger.Println("UpdateQCHigh Failed")
	return false
}

// OnReceiveProposal handles a replica's response to the Proposal from the leader
func (hs *HotStuff) OnReceiveProposal(block *Block) (*PartialCert, error) {
	logger.Println("OnReceiveProposal: ", block)
	hs.blocks.Put(block)

	// wake anyone waiting for a proposal
	defer hs.Signal()

	qcBlock, nExists := hs.expectBlockFor(block.Justify)

	hs.mut.Lock()

	if block.Height <= hs.vHeight {
		hs.mut.Unlock()
		logger.Println("OnReceiveProposal: Block height less than vHeight")
		return nil, fmt.Errorf("Block was not accepted")
	}

	safe := false
	if nExists && qcBlock.Height > hs.bLock.Height {
		safe = true
	} else {
		logger.Println("OnReceiveProposal: liveness condition failed")
		// check if block extends bLock
		b := block
		ok := true
		for ok && b.Height > hs.bLock.Height+1 {
			b, ok = hs.blocks.Get(b.ParentHash)
		}
		if ok && b.ParentHash == hs.bLock.Hash() {
			safe = true
		} else {
			logger.Println("OnReceiveProposal: safety condition failed")
		}
	}

	if !safe {
		hs.mut.Unlock()
		logger.Println("OnReceiveProposal: Block not safe")
		return nil, fmt.Errorf("Block was not accepted")
	}

	logger.Println("OnReceiveProposal: Accepted block")
	hs.vHeight = block.Height

	hs.mut.Unlock()

	// queue block for update
	hs.pendingUpdates <- block

	pc, err := hs.SigCache.CreatePartialCert(hs.id, hs.privKey, block)
	if err != nil {
		return nil, err
	}

	// remove all commands associated with this block from the pending commands
	hs.cmdCache.MarkProposed(block.Commands...)

	hs.pmNotify(Notification{ReceiveProposal, block, hs.qcHigh})
	return pc, nil
}

// OnReceiveNewView handles the leader's response to receiving a NewView rpc from a replica
func (hs *HotStuff) OnReceiveNewView(qc *QuorumCert) {
	logger.Println("OnReceiveNewView")
	hs.UpdateQCHigh(qc)
	block, _ := hs.blocks.BlockOf(qc)
	hs.pmNotify(Notification{ReceiveNewView, block, qc})
}

func (hs *HotStuff) updateAsync(ctx context.Context) {
	for {
		select {
		case n := <-hs.pendingUpdates:
			hs.update(n)
		case <-ctx.Done():
			return
		}
	}
}

func (hs *HotStuff) update(block *Block) {
	// block1 = b'', block2 = b', block3 = b
	block1, ok := hs.blocks.BlockOf(block.Justify)
	if !ok || block1.Committed {
		return
	}

	logger.Println("PRE COMMIT: ", block1)
	// PRE-COMMIT on block1
	hs.UpdateQCHigh(block.Justify)

	block2, ok := hs.blocks.BlockOf(block1.Justify)
	if !ok || block2.Committed {
		return
	}

	hs.mut.Lock()
	defer hs.mut.Unlock()

	if block2.Height > hs.bLock.Height {
		hs.bLock = block2 // COMMIT on block2
		logger.Println("COMMIT: ", block2)
	}

	block3, ok := hs.blocks.BlockOf(block2.Justify)
	if !ok || block3.Committed {
		return
	}

	if block1.ParentHash == block2.Hash() && block2.ParentHash == block3.Hash() {
		logger.Println("DECIDE ", block3)
		hs.commit(block3)
		hs.bExec = block3 // DECIDE on block3
	}

	// Free up space by deleting old data
	hs.blocks.GarbageCollectBlocks(hs.GetVotedHeight())
	hs.cmdCache.TrimToLen(hs.BatchSize * 5)
	hs.SigCache.EvictOld(hs.QuorumSize * 5)
}

func (hs *HotStuff) commit(block *Block) {
	// only called from within update. Thus covered by its mutex lock.
	if hs.bExec.Height < block.Height {
		if parent, ok := hs.blocks.ParentOf(block); ok {
			hs.commit(parent)
		}
		block.Committed = true
		logger.Println("EXEC ", block)
		hs.Exec(block.Commands)
	}
}

// Propose will fetch a command from the Commands channel and proposes it to the other replicas
func (hs *HotStuff) Propose(ctx context.Context) {
	cmds := hs.cmdCache.GetFirst(hs.BatchSize)

	logger.Printf("Propose (%d commands)\n", len(cmds))
	hs.mut.Lock()

	newBlock := CreateLeaf(hs.bLeaf, cmds, hs.qcHigh, hs.bLeaf.Height+1)
	hs.pmNotify(Notification{Propose, newBlock, hs.qcHigh})

	hs.mut.Unlock()

	// async self vote
	selfVote := make(chan *PartialCert)
	go func() {
		partialCert, err := hs.OnReceiveProposal(newBlock)
		if err != nil {
			panic(fmt.Errorf("Failed to vote for own proposal: %w", err))
		}
		selfVote <- partialCert
	}()

	qc, err := hs.DoPropose(ctx, newBlock)

	if err != nil {
		logger.Println("ProposeQC finished with error: ", err)
		return
	}

	err = qc.AddPartial(<-selfVote)
	if err != nil {
		logger.Println("Failed to add own vote to QC: ", err)
	}

	hs.UpdateQCHigh(qc)

	hs.pmNotify(Notification{QCFinish, newBlock, qc})
}

// SendNewView sends a newView message to the leader replica.
func (hs *HotStuff) SendNewView(ctx context.Context, leader ReplicaID) error {
	logger.Println("SendNewView")
	replica := hs.Replicas[leader]
	if replica == nil {
		panic(fmt.Errorf("Could not find replica with id '%d'", leader))
	}
	if replica.ID == hs.id {
		// "send" new-view to self
		hs.pmNotify(Notification{ReceiveNewView, hs.bLeaf, hs.qcHigh})
		return nil
	}
	err := hs.DoNewView(ctx, leader, hs.GetQCHigh())
	if err != nil {
		logger.Println("Failed to send new view to leader: ", err)
		return err
	}
	return nil
}

// Close frees resources held by HotStuff and closes backend connections
func (hs *HotStuff) Close() {
	hs.cancel()
	hs.Backend.Close()
}

// CreateLeaf returns a new block that extends the parent.
func CreateLeaf(parent *Block, cmds []Command, qc *QuorumCert, height int) *Block {
	return &Block{
		ParentHash: parent.Hash(),
		Commands:   cmds,
		Justify:    qc,
		Height:     height,
	}
}
