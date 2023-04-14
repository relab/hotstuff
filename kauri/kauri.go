// Package kauri contains the implementation of the kauri protocol
package kauri

import (
	"context"
	"encoding/binary"
	"errors"
	"math/rand"
	"reflect"
	"sort"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/proto/kauripb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("kauri", New)
}

// TreeType determines the type of tree to be built, for future extension
type TreeType int

// NoFaultTree is a tree without any faulty replicas in the tree, which is currently supported.
// TreeWithFaults is a tree with faulty replicas, not yet implemented.
const (
	NoFaultTree    TreeType = 1
	TreeWithFaults TreeType = 2
)

// Kauri structure contains the modules for kauri protocol implementation.
type Kauri struct {
	configuration           *backend.Config
	server                  *backend.Server
	blockChain              modules.BlockChain
	crypto                  modules.Crypto
	eventLoop               *eventloop.EventLoop
	logger                  logging.Logger
	opts                    *modules.Options
	synchronizer            modules.Synchronizer
	leaderRotation          modules.LeaderRotation
	tree                    TreeConfiguration
	initDone                bool
	aggregatedContributions map[hotstuff.ChainNumber]hotstuff.QuorumSignature
	blockHashes             map[hotstuff.ChainNumber]hotstuff.Hash
	currentViews            map[hotstuff.ChainNumber]hotstuff.View
	senders                 map[hotstuff.ChainNumber][]hotstuff.ID
	nodes                   map[hotstuff.ID]*kauripb.Node
	isAggregationSent       map[hotstuff.ChainNumber]bool
}

// New initializes the kauri structure
func New() modules.Kauri {
	return &Kauri{nodes: make(map[hotstuff.ID]*kauripb.Node)}
}

// InitModule initializes the Handel module.
func (k *Kauri) InitModule(mods *modules.Core) {
	mods.Get(
		&k.configuration,
		&k.server,
		&k.blockChain,
		&k.crypto,
		&k.eventLoop,
		&k.logger,
		&k.opts,
		&k.leaderRotation,
		&k.synchronizer,
	)
	k.opts.SetShouldUseKauri()
	k.eventLoop.RegisterObserver(backend.ConnectedEvent{}, func(_ any) {
		k.postInit()
	})
	k.eventLoop.RegisterHandler(ContributionRecvEvent{}, func(event any) {
		k.OnContributionRecv(event.(ContributionRecvEvent))
	})
}

func (k *Kauri) postInit() {
	k.logger.Info("Kauri: Initializing")
	kauripb.RegisterKauriServer(k.server.GetGorumsServer(), serviceImpl{k})
	k.initializeConfiguration()
}

func (k *Kauri) initializeConfiguration() {
	kauriCfg := kauripb.ConfigurationFromRaw(k.configuration.GetRawConfiguration(), nil)
	for _, n := range kauriCfg.Nodes() {
		k.nodes[hotstuff.ID(n.ID())] = n
	}
	k.tree = CreateTree(k.configuration.Len(), k.opts.ID())

	// pIDs := make(map[hotstuff.ID]int)
	// for id := range k.configuration.ActiveReplicas() {
	// 	pIDs[id] = index
	// 	index++
	// }
	idMappings := make(map[hotstuff.ID]int)
	for i := 0; i < k.configuration.Len(); i++ {
		idMappings[hotstuff.ID(i+1)] = i
	}
	k.tree.InitializeWithPIDs(idMappings)
	k.initDone = true
	k.senders = make(map[hotstuff.ChainNumber][]hotstuff.ID)
	k.isAggregationSent = make(map[hotstuff.ChainNumber]bool)
	k.blockHashes = make(map[hotstuff.ChainNumber]hotstuff.Hash)
	k.currentViews = make(map[hotstuff.ChainNumber]hotstuff.View)
	k.aggregatedContributions = make(map[hotstuff.ChainNumber]hotstuff.QuorumSignature)
}

// Begin starts dissemination of proposal and aggregation of votes.
func (k *Kauri) Begin(pc hotstuff.PartialCert, p hotstuff.ProposeMsg) {
	if !k.initDone {
		k.eventLoop.DelayUntil(backend.ConnectedEvent{}, func() { k.Begin(pc, p) })
		return
	}
	k.reset(pc.ChainNumber())
	k.blockHashes[pc.ChainNumber()] = pc.BlockHash()
	k.currentViews[pc.ChainNumber()] = p.Block.View()
	k.aggregatedContributions[pc.ChainNumber()] = pc.Signature()
	// ids := k.randomizeIDS(k.blockHash, k.leaderRotation.GetLeader(k.currentView))
	// k.tree.InitializeWithPIDs(ids)
	k.SendProposalToChildren(p)
	waitTime := time.Duration(uint64(k.tree.GetHeight() * 20 * int(time.Millisecond)))
	go k.aggregateAndSend(waitTime, k.currentViews[pc.ChainNumber()], pc.ChainNumber())
}

func (k *Kauri) reset(chainNumber hotstuff.ChainNumber) {
	//delete(k.aggregatedContributions, chainNumber)
	delete(k.senders, chainNumber)
	k.aggregatedContributions[chainNumber] = nil
	k.senders[chainNumber] = make([]hotstuff.ID, 0)
	k.isAggregationSent[chainNumber] = false
}

func (k *Kauri) aggregateAndSend(t time.Duration, view hotstuff.View, chainNumber hotstuff.ChainNumber) {
	ticker := time.NewTicker(t)
	<-ticker.C
	ticker.Stop()
	if k.currentViews[chainNumber] != view {
		return
	}
	if !k.isAggregationSent[chainNumber] {
		k.SendContributionToParent(chainNumber)
	}
}

// SendProposalToChildren sends the proposal to the children
func (k *Kauri) SendProposalToChildren(p hotstuff.ProposeMsg) {
	children := k.tree.GetChildren()
	if len(children) != 0 {
		config, err := k.configuration.SubConfig(k.tree.GetChildren())
		if err != nil {
			k.logger.Error("Unable to send the proposal to children", err)
			return
		}
		k.logger.Debug("sending proposal to children ", k.tree.GetChildren())
		config.Propose(p)
	} else {
		k.SendContributionToParent(p.Block.ChainNumber())
		k.isAggregationSent[p.Block.ChainNumber()] = true
	}
}

// OnContributionRecv is invoked upon receiving the vote for aggregation.
func (k *Kauri) OnContributionRecv(event ContributionRecvEvent) {
	contribution := event.Contribution
	chainNumber := hotstuff.ChainNumber(contribution.ChainNumber)
	if k.currentViews[chainNumber] !=
		hotstuff.View(contribution.View) {
		return
	}
	k.logger.Debug("processing the contribution from ", contribution.ID)
	currentSignature := hotstuffpb.QuorumSignatureFromProto(contribution.Signature)
	_, err := k.mergeWithContribution(currentSignature, chainNumber)
	if err != nil {
		k.logger.Debug("Unable to merge the contribution from ", contribution.ID)
		return
	}
	k.senders[chainNumber] = append(k.senders[chainNumber], hotstuff.ID(contribution.ID))
	if isSubSet(k.tree.GetSubTreeNodes(), k.senders[chainNumber]) {
		k.SendContributionToParent(chainNumber)
		k.isAggregationSent[chainNumber] = true
	}
}

// SendContributionToParent sends contribution to the parent node.
func (k *Kauri) SendContributionToParent(chainNumber hotstuff.ChainNumber) {
	parent, ok := k.tree.GetParent()
	if ok {
		node, isPresent := k.nodes[parent]
		if isPresent {
			node.SendContribution(context.Background(), &kauripb.Contribution{
				ID:          uint32(k.opts.ID()),
				Signature:   hotstuffpb.QuorumSignatureToProto(k.aggregatedContributions[chainNumber]),
				View:        uint64(k.currentViews[chainNumber]),
				ChainNumber: uint32(chainNumber),
			})
		}
	}
}

type serviceImpl struct {
	k *Kauri
}

func (i serviceImpl) SendContribution(ctx gorums.ServerCtx, request *kauripb.Contribution) {
	i.k.eventLoop.AddEvent(ContributionRecvEvent{Contribution: request})
}

// ContributionRecvEvent is raised when a contribution is received.
type ContributionRecvEvent struct {
	Contribution *kauripb.Contribution
}

func (k *Kauri) canMergeContributions(a, b hotstuff.QuorumSignature) bool {
	canMerge := true
	if a == nil || b == nil {
		k.logger.Info("one of it is nil")
		return false
	}
	a.Participants().RangeWhile(func(i hotstuff.ID) bool {
		b.Participants().RangeWhile(func(j hotstuff.ID) bool {
			// cannot merge a and b if they both contain a contribution from the same ID.
			if i == j {
				canMerge = false
			}
			return canMerge
		})
		return canMerge
	})
	return canMerge
}

func (k *Kauri) verifyContribution(signature hotstuff.QuorumSignature, hash hotstuff.Hash, chainNumber hotstuff.ChainNumber) bool {
	verified := false
	block, ok := k.blockChain.Get(chainNumber, hash)
	if !ok {
		k.logger.Info("failed to fetch the block ", hash)
		return verified
	}
	verified = k.crypto.Verify(signature, block.ToBytes())
	return verified
}

func (k *Kauri) mergeWithContribution(currentSignature hotstuff.QuorumSignature, chainNumber hotstuff.ChainNumber) (bool, error) {
	isVerified := k.verifyContribution(currentSignature, k.blockHashes[chainNumber], chainNumber)
	if !isVerified {
		k.logger.Info("Contribution verification failed for view ", k.currentViews[chainNumber],
			"from participants", currentSignature.Participants(), " block hash ", k.blockHashes[chainNumber])
		return false, errors.New("unable to verify the contribution")
	}
	if k.aggregatedContributions[chainNumber] == nil {
		k.aggregatedContributions[chainNumber] = currentSignature
		return false, nil
	}

	if k.canMergeContributions(currentSignature, k.aggregatedContributions[chainNumber]) {
		new, err := k.crypto.Combine(currentSignature, k.aggregatedContributions[chainNumber])
		if err == nil {
			k.aggregatedContributions[chainNumber] = new
			if new.Participants().Len() >= k.configuration.QuorumSize() {
				k.logger.Debug("Aggregated Complete QC and sending the event")
				k.eventLoop.AddEvent(hotstuff.NewViewMsg{
					SyncInfo: hotstuff.NewSyncInfo(chainNumber).WithQC(hotstuff.NewQuorumCert(
						k.aggregatedContributions[chainNumber],
						k.currentViews[chainNumber],
						k.blockHashes[chainNumber],
						chainNumber,
					)),
				})
				return true, nil
			}
		} else {
			k.logger.Info("Failed to combine signatures: %v", err)
			return false, errors.New("unable to combine signature")
		}
	} else {
		k.logger.Debug("Failed to merge signatures due to overlap of signatures.")
		return false, errors.New("unable to merge signature")
	}
	return false, nil
}

func isSubSet(a, b []hotstuff.ID) bool {
	c := hotstuff.NewIDSet()
	for _, id := range b {
		c.Add(id)
	}
	for _, id := range a {
		if !c.Contains(id) {
			return false
		}
	}
	return true
}

func (k *Kauri) randomizeIDS(hash hotstuff.Hash, leaderID hotstuff.ID) map[hotstuff.ID]int {
	//assign leader to the root of the tree.

	totalNodes := k.configuration.Len()
	ids := make([]hotstuff.ID, 0, totalNodes)
	for id := range k.configuration.Replicas() {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	seed := k.opts.SharedRandomSeed() + int64(binary.LittleEndian.Uint64(hash[:]))
	//Shuffle the list of IDs using the shared random seed + the first 8 bytes of the hash.
	rnd := rand.New(rand.NewSource(seed))
	rnd.Shuffle(len(ids), reflect.Swapper(ids))
	lIndex := 0
	for index, id := range ids {
		if id == leaderID {
			lIndex = index
		}
	}
	currentRoot := ids[0]
	ids[0] = ids[lIndex]
	ids[lIndex] = currentRoot
	posMapping := make(map[hotstuff.ID]int)
	for index, ID := range ids {
		posMapping[ID] = index
	}
	return posMapping
}
