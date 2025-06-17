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
	blockChain *blockchain.BlockChain
	viewStates *protocol.ViewStates
	ruler      modules.CommitRuler
}

func NewCommitter(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	blockChain *blockchain.BlockChain,
	viewStates *protocol.ViewStates,
	rules modules.CommitRuler,
) *Committer {
	return &Committer{
		eventLoop:  eventLoop,
		blockChain: blockChain,
		ruler:      rules,
		logger:     logger,
		viewStates: viewStates,
	}
}

// Stores the block before further execution.
func (cm *Committer) commit(block *hotstuff.Block) error {
	err := cm.commitInner(block, cm.viewStates.CommittedBlock())
	if err != nil {
		return err
	}

	forkedBlocks := cm.blockChain.PruneToHeight(
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

func (cm *Committer) Update(block *hotstuff.Block) {
	cm.logger.Debugf("Update: %v", block)
	cm.blockChain.Store(block)
	// NOTE: this overwrites the block variable. If it was nil, simply don't commit.
	if block = cm.ruler.CommitRule(block); block != nil {
		err := cm.commit(block) // committer will eventually execute the command.
		if err != nil {
			cm.logger.Warnf("failed to commit: %v", err)
		}
	}
}

// recursive helper for commit
func (cm *Committer) commitInner(block, committedBlock *hotstuff.Block) error {
	if committedBlock.View() >= block.View() {
		return nil
	}
	if parent, ok := cm.blockChain.Get(block.Parent()); ok {
		err := cm.commitInner(parent, committedBlock)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.Parent())
	}
	cm.logger.Debug("EXEC: ", block)
	batch := block.Commands()
	cm.eventLoop.AddEvent(hotstuff.CommitEvent{Block: block})
	cm.eventLoop.AddEvent(clientpb.ExecuteEvent{Batch: batch})
	cm.eventLoop.AddEvent(hotstuff.ConsensusLatencyEvent{Latency: time.Since(block.Timestamp())})
	cm.viewStates.UpdateCommittedBlock(block)
	return nil
}
