package dependencies

import (
	"github.com/relab/hotstuff/internal/latency"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/kauri"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/synchronizer"
)

type modSetProtocol struct {
	ConsensusRules modules.ConsensusRules
	Kauri          modules.Kauri
	LeaderRotation modules.LeaderRotation
	ViewDuration   modules.ViewDuration
}

type ViewDurationOptions struct {
	SampleSize   uint64
	StartTimeout float64
	MaxTimeout   float64
	Multiplier   float64
}

func newProtocolModules(
	depsCore *DepSetCore,
	depsNet *DepSetNetwork,
	depsSecure *DepSetSecurity,
	depsSrv *DepSetService,

	// TODO: avoid modifying this so it doesn't depend on orchestrationpb
	opts *orchestrationpb.ReplicaOpts,
	// TODO: consider using the ReplicaOpts instead of passing names individually.
	consensusName,
	leaderRotationName,
	byzantineStrategy string,
	vdOpt ViewDurationOptions,
) (*modSetProtocol, error) {
	consensusRules, err := newConsensusRulesModule(consensusName, depsSecure.BlockChain, depsCore.Logger, depsCore.Options)
	if err != nil {
		return nil, err
	}
	leaderRotation, err := newLeaderRotationModule(
		leaderRotationName,
		consensusRules.ChainLength(),
		depsSecure.BlockChain,
		depsNet.Config,
		depsSrv.Committer,
		depsCore.Logger,
		depsCore.Options,
	)
	if err != nil {
		return nil, err
	}

	var kauriOptional modules.Kauri = nil

	if opts.GetKauri() {
		kauriOptional = kauri.New(
			depsSecure.CryptoImpl,
			leaderRotation,
			depsSecure.BlockChain,
			depsCore.Options,
			depsCore.EventLoop,
			depsNet.Config,
			depsNet.Sender,
			depsCore.Logger,
		)
	}

	var duration modules.ViewDuration
	if leaderRotationName == leaderrotation.TreeLeaderModuleName {
		// TODO(meling): Temporary default; should be configurable and moved to the appropriate place.
		opts.SetTreeHeightWaitTime()
		// create tree only if we are using tree leader (Kauri)
		depsCore.Options.SetTree(createTree(opts))
		duration = synchronizer.NewFixedViewDuration(opts.GetInitialTimeout().AsDuration())
	} else {
		duration = synchronizer.NewViewDuration(
			vdOpt.SampleSize, vdOpt.StartTimeout, vdOpt.MaxTimeout, vdOpt.Multiplier,
		)
	}
	if byzantineStrategy != "" {
		byz, err := newByzantineStrategyModule(
			byzantineStrategy,
			consensusRules,
			depsSecure.BlockChain,
			depsCore.Options)
		if err != nil {
			return nil, err
		}
		consensusRules = byz
		depsCore.Logger.Infof("assigned byzantine strategy: %s", byzantineStrategy)
	}
	return &modSetProtocol{
		ConsensusRules: consensusRules,
		Kauri:          kauriOptional,
		LeaderRotation: leaderRotation,
		ViewDuration:   duration,
	}, nil
}

type DepSetProtocol struct {
	Consensus    *consensus.Consensus
	Synchronizer *synchronizer.Synchronizer
}

func NewProtocol(
	depsCore *DepSetCore,
	depsNet *DepSetNetwork,
	depsSecure *DepSetSecurity,
	depsSrv *DepSetService,

	opts *orchestrationpb.ReplicaOpts,
	consensusName,
	leaderRotationName,
	byzantineStrategy string,
	vdOpt ViewDurationOptions,
) (*DepSetProtocol, error) {
	mods, err := newProtocolModules(
		depsCore,
		depsNet,
		depsSecure,
		depsSrv,
		opts,
		consensusName,
		leaderRotationName,
		byzantineStrategy,
		vdOpt,
	)
	if err != nil {
		return nil, err
	}
	csus := consensus.New(
		mods.ConsensusRules,
		mods.LeaderRotation,
		mods.Kauri,
		depsSecure.BlockChain,
		depsSrv.Committer,
		depsSrv.CmdCache,
		depsNet.Sender,
		depsSecure.CertAuth,
		depsNet.Config,
		depsCore.EventLoop,
		depsCore.Logger,
		depsCore.Options,
	)
	synch := synchronizer.New(
		depsSecure.CryptoImpl,
		mods.LeaderRotation,
		mods.ViewDuration,
		depsSecure.BlockChain,
		csus,
		depsSecure.CertAuth,
		depsNet.Config,
		depsNet.Sender,
		depsCore.EventLoop,
		depsCore.Logger,
		depsCore.Options,
	)
	return &DepSetProtocol{
		Consensus:    csus,
		Synchronizer: synch,
	}, nil
}

// createTree creates a tree based on the given replica options.
func createTree(replicaOpts *orchestrationpb.ReplicaOpts) tree.Tree {
	tree := tree.CreateTree(replicaOpts.HotstuffID(), int(replicaOpts.GetBranchFactor()), replicaOpts.TreePositionIDs())
	switch {
	case replicaOpts.GetAggregationTime():
		tree.SetAggregationWaitTime(latency.MatrixFrom(replicaOpts.GetLocations()), replicaOpts.TreeDeltaDuration())
	case replicaOpts.GetTreeHeightTime():
		fallthrough
	default:
		tree.SetTreeHeightWaitTime(replicaOpts.TreeDeltaDuration())
	}
	return tree
}
