package twins

import (
	"fmt"
	"strings"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto"
	"github.com/relab/hotstuff/security/crypto/keygen"
	"github.com/relab/hotstuff/wiring"
)

type node struct {
	config         *core.RuntimeConfig
	logger         logging.Logger
	sender         *emulatedSender
	blockchain     *blockchain.Blockchain
	commandCache   *clientpb.CommandCache
	voter          *consensus.Voter
	proposer       *consensus.Proposer
	eventLoop      *eventloop.EventLoop
	viewStates     *protocol.ViewStates
	leaderRotation leaderrotation.LeaderRotation
	synchronizer   *synchronizer.Synchronizer
	timeoutManager *timeoutManager

	id             NodeID
	executedBlocks []*hotstuff.Block
	effectiveView  hotstuff.View
	log            strings.Builder
}

func newNode(n *Network, nodeID NodeID, consensusName string) (*node, error) {
	cryptoName := crypto.NameECDSA
	pk, err := keygen.GenerateECDSAPrivateKey()
	if err != nil {
		return nil, err
	}
	node := &node{
		id:           nodeID,
		config:       core.NewRuntimeConfig(nodeID.ReplicaID, pk, core.WithSyncVerification()),
		commandCache: clientpb.NewCommandCache(1),
	}
	node.logger = logging.NewWithDest(&node.log, fmt.Sprintf("r%dn%d", nodeID.ReplicaID, nodeID.NetworkID))
	node.eventLoop = eventloop.New(node.logger, 100)
	node.sender = newSender(n, node)
	base, err := crypto.New(
		node.config,
		cryptoName,
	)
	if err != nil {
		return nil, err
	}
	depsSecurity := wiring.NewSecurity(
		node.eventLoop,
		node.logger,
		node.config,
		node.sender,
		base,
		cert.WithCache(100),
	)
	node.blockchain = depsSecurity.BlockChain()
	consensusRules, err := rules.New(node.logger, node.config, node.blockchain, consensusName)
	if err != nil {
		return nil, err
	}
	node.viewStates, err = protocol.NewViewStates(node.blockchain, depsSecurity.Authority())
	if err != nil {
		return nil, err
	}
	committer := consensus.NewCommitter(node.eventLoop, node.logger, node.blockchain, node.viewStates, consensusRules)
	node.leaderRotation = leaderRotation(n.views)
	votingMachine := votingmachine.New(
		node.logger,
		node.eventLoop,
		node.config,
		depsSecurity.BlockChain(),
		depsSecurity.Authority(),
		node.viewStates,
	)
	comm := comm.NewClique(
		node.config,
		votingMachine,
		node.leaderRotation,
		node.sender,
	)
	node.voter = consensus.NewVoter(
		node.config,
		node.leaderRotation,
		consensusRules,
		comm,
		depsSecurity.Authority(),
		committer,
	)
	node.proposer = consensus.NewProposer(
		node.eventLoop,
		node.config,
		node.blockchain,
		node.viewStates,
		consensusRules,
		comm,
		node.voter,
		node.commandCache,
		committer,
	)
	node.synchronizer = synchronizer.New(
		node.eventLoop,
		node.logger,
		node.config,
		depsSecurity.Authority(),
		node.leaderRotation,
		viewduration.NewFixed(100*time.Millisecond),
		node.proposer,
		node.voter,
		node.viewStates,
		node.sender,
	)
	node.timeoutManager = newTimeoutManager(n, node, node.eventLoop, node.viewStates)
	// necessary to count executed commands.
	node.eventLoop.RegisterHandler(hotstuff.CommitEvent{}, func(event any) {
		commit := event.(hotstuff.CommitEvent)
		node.executedBlocks = append(node.executedBlocks, commit.Block)
	})
	commandGenerator := &commandGenerator{}
	for range n.views {
		cmd := commandGenerator.next()
		node.commandCache.Add(cmd)
	}
	return node, nil
}
