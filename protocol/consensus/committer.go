package consensus

import (
	"fmt"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/security/blockchain"
)

// Committer commits the correct block for a view.
type Committer struct {
	eventLoop  *eventloop.EventLoop
	logger     logging.Logger
	blockchain *blockchain.Blockchain
	viewStates *protocol.ViewStates
	ruler      modules.CommitRuler
}

func NewCommitter(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	blockchain *blockchain.Blockchain,
	viewStates *protocol.ViewStates,
	ruler modules.CommitRuler,
) *Committer {
	return &Committer{
		eventLoop:  eventLoop,
		blockchain: blockchain,
		ruler:      ruler,
		logger:     logger,
		viewStates: viewStates,
	}
}

// TryCommit stores the given block in the local blockchain and applies the
// CommitRule to identify the youngest ancestor block eligible to be committed.
// This eligible block is then used as the starting point for recursively
// committing its uncommitted ancestor blocks.
func (cm *Committer) TryCommit(block *hotstuff.Block) error {
	cm.logger.Debugf("TryCommit: %v", block)
	cm.blockchain.Store(block)
	// NOTE: this overwrites the block variable. If it was nil, simply don't commit.
	if block = cm.ruler.CommitRule(block); block != nil {
		// recursively commit the block's ancestors before committing the block itself
		if err := cm.commit(block); err != nil {
			return fmt.Errorf("failed to commit: %w", err)
		}
	}
	return nil
}

// Stores the block before further execution.
func (cm *Committer) commit(block *hotstuff.Block) error {
	err := cm.commitInner(block, cm.viewStates.CommittedBlock())
	if err != nil {
		return err
	}

	forkedBlocks := cm.blockchain.PruneToHeight(
		cm.viewStates.CommittedBlock().View(),
		block.View(),
	)
	for _, block := range forkedBlocks {
		cm.eventLoop.AddEvent(clientpb.AbortEvent{
			Batch: block.Commands(),
		})
	}
	return nil
}

// recursive helper for commit
func (cm *Committer) commitInner(block, committedBlock *hotstuff.Block) error {
	if committedBlock.View() >= block.View() {
		return nil
	}
	if parent, ok := cm.blockchain.Get(block.Parent()); ok {
		err := cm.commitInner(parent, committedBlock)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}
	cm.logger.Debug("EXEC: ", block)
	batch := block.Commands()
	// CommitEvent holds the entire block and is used in twins since it needs the hash.
	cm.eventLoop.AddEvent(hotstuff.CommitEvent{Block: block})
	// ExecuteEvent is a solution to cyclic dependencies between hotstuff package and clientpb.
	cm.eventLoop.AddEvent(clientpb.ExecuteEvent{Batch: batch})
	cm.eventLoop.AddEvent(hotstuff.ConsensusLatencyEvent{Latency: time.Since(block.Timestamp())})
	cm.viewStates.UpdateCommittedBlock(block)
	return nil
}
