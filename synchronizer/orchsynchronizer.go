package synchronizer

import (
	"context"
	"fmt"

	"github.com/relab/hotstuff/consensus"
)

// orchestrators currently only propose on newreconfiguration request.
// View changes only upon the new request
// TimeoOut is not currently supported
// It is expected that clients do not reconfigure unless the previous request processing
// is completed.
type OrchSynchronizer struct {
	mods        *consensus.Modules
	currentView consensus.View
	highQC      consensus.QuorumCert
	leafBlock   *consensus.Block
	viewCtx     context.Context // a context that is cancelled at the end of the current view
	reconfigReq consensus.ReconfigurationEvent
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (s *OrchSynchronizer) InitConsensusModule(mods *consensus.Modules, opts *consensus.OptionsBuilder) {
	s.mods = mods
	s.mods.EventLoop().RegisterHandler(consensus.ReconfigurationEvent{}, func(event interface{}) {
		reconfigEvent := event.(consensus.ReconfigurationEvent)
		s.OnReconfigEvent(reconfigEvent)
	})
	s.mods.EventLoop().RegisterHandler(consensus.NewViewMsg{}, func(event interface{}) {
		newViewMsg := event.(consensus.NewViewMsg)
		s.OnNewView(newViewMsg)
	})
	var err error
	s.highQC, err = s.mods.Crypto().CreateQuorumCert(consensus.GetGenesis(), []consensus.PartialCert{})
	if err != nil {
		panic(fmt.Errorf("unable to create empty quorum cert for genesis block: %v", err))
	}
}

func (s *OrchSynchronizer) OnRestart(syncInfo consensus.SyncInfo) {

}
func NewOrchSynchronizer() consensus.Synchronizer {
	ctx, _ := context.WithCancel(context.Background())
	return &OrchSynchronizer{
		leafBlock:   consensus.GetGenesis(),
		currentView: 1,
		viewCtx:     ctx,
	}
}
func (s *OrchSynchronizer) Stop() {

}

func (s *OrchSynchronizer) SyncInfo() consensus.SyncInfo {
	return consensus.NewSyncInfo()
}

// HighQC returns the highest known QC.
func (s *OrchSynchronizer) HighQC() consensus.QuorumCert {
	return s.highQC
}

// LeafBlock returns the current leaf block.
func (s *OrchSynchronizer) LeafBlock() *consensus.Block {
	return s.leafBlock
}

// View returns the current view.
func (s *OrchSynchronizer) View() consensus.View {
	return s.currentView
}

// ViewContext returns a context that is cancelled at the end of the view.
func (s *OrchSynchronizer) ViewContext() context.Context {
	return s.viewCtx
}

func (s *OrchSynchronizer) Start(ctx context.Context) {
	go func() {
		<-ctx.Done()
		//TODO add code for clean up
	}()
}

func (s *OrchSynchronizer) AdvanceView(syncInfo consensus.SyncInfo) {
	if qc, ok := syncInfo.QC(); ok {
		if !s.mods.Crypto().VerifyQuorumCert(qc) {
			s.mods.Logger().Info("Quorum Certificate could not be verified!")
			return
		}
		s.UpdateHighQC(qc)
	}
	// View is advanced only on the reconfiguration request
}

func (s *OrchSynchronizer) UpdateHighQC(qc consensus.QuorumCert) {
	s.mods.Logger().Debugf("updateHighQC: %v", qc)
	if !s.mods.Crypto().VerifyQuorumCert(qc) {
		s.mods.Logger().Info("updateHighQC: QC could not be verified!")
		return
	}

	newBlock, ok := s.mods.BlockChain().Get(qc.BlockHash())
	if !ok {
		s.mods.Logger().Info("updateHighQC: Could not find block referenced by new QC!")
		return
	}

	oldBlock, ok := s.mods.BlockChain().Get(s.highQC.BlockHash())
	if !ok {
		s.mods.Logger().Panic("Block from the old highQC missing from chain")
	}

	if newBlock.View() > oldBlock.View() {
		s.mods.Logger().Debug("HighQC updated")
		s.highQC = qc
		s.leafBlock = newBlock
	}
}

func (orchSync *OrchSynchronizer) OnReconfigEvent(event consensus.ReconfigurationEvent) {
	orchSync.currentView += 1
	orchSync.reconfigReq = event
	syncInfo := consensus.NewSyncInfo().WithQC(orchSync.highQC)
	orchSync.mods.Consensus().Propose(syncInfo)
}

func (orchSync *OrchSynchronizer) OnNewView(v consensus.NewViewMsg) {
	// Now the reconfig request is replicated, invoke reconfig start on the active configuration.
	leader := orchSync.mods.LeaderRotation().GetLeader(orchSync.currentView)
	if leader == orchSync.mods.ID() {
		orchSync.mods.EventLoop().AddEvent(consensus.ReconfigurationStartEvent{
			Replicas: orchSync.reconfigReq.Replicas})
	}
}

var _ consensus.Synchronizer = (*OrchSynchronizer)(nil)
