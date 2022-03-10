// Package backend implements the networking backend for hotstuff using the Gorums framework.
package backend

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/proto/reconfigurationpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Replica provides methods used by hotstuff to send messages to replicas.
type Replica struct {
	node             *hotstuffpb.Node
	orchestratorNode *reconfigurationpb.Node
	id               hotstuff.ID
	pubKey           consensus.PublicKey
	voteCancel       context.CancelFunc
	newviewCancel    context.CancelFunc
	state            hotstuff.ReplicaState
	address          string
}

// ID returns the replica's ID.
func (r *Replica) ID() hotstuff.ID {
	return r.id
}

// PublicKey returns the replica's public key.
func (r *Replica) PublicKey() consensus.PublicKey {
	return r.pubKey
}

func (r *Replica) GetAddress() string {
	return r.address
}

// Vote sends the partial certificate to the other replica.
func (r *Replica) Vote(cert consensus.PartialCert) {
	if r.node == nil {
		return
	}
	var ctx context.Context
	r.voteCancel()
	ctx, r.voteCancel = context.WithCancel(context.Background())
	pCert := hotstuffpb.PartialCertToProto(cert)
	r.node.Vote(ctx, pCert, gorums.WithNoSendWaiting())
}

// NewView sends the quorum certificate to the other replica.
func (r *Replica) NewView(msg consensus.SyncInfo) {
	if r.node == nil {
		return
	}
	var ctx context.Context
	r.newviewCancel()
	ctx, r.newviewCancel = context.WithCancel(context.Background())
	r.node.NewView(ctx, hotstuffpb.SyncInfoToProto(msg), gorums.WithNoSendWaiting())
}

func (r *Replica) IsActive() bool {
	return r.state == hotstuff.Active
}

func (r *Replica) IsRead() bool {
	return r.state == hotstuff.Read
}

func (r *Replica) IsLearn() bool {
	return r.state == hotstuff.Learn
}

func (r *Replica) IsOrchestrator() bool {
	return r.state == hotstuff.Orchestrator
}

func (r *Replica) SetReplicaState(newState hotstuff.ReplicaState) {
	r.state = newState
}

// Config holds information about the current configuration of replicas that participate in the protocol,
// and some information about the local replica. It also provides methods to send messages to the other replicas.
type Config struct {
	sync.Mutex
	mods               *consensus.Modules
	optsPtr            *[]gorums.ManagerOption // using a pointer so that options can be GCed after initialization
	mgr                *hotstuffpb.Manager
	activeConfig       *hotstuffpb.Configuration
	readConfigMgr      *hotstuffpb.Manager
	readConfig         *hotstuffpb.Configuration
	learnConfigMgr     *hotstuffpb.Manager
	learnConfig        *hotstuffpb.Configuration
	orchestratorConfig *reconfigurationpb.Configuration
	reconfigManager    *reconfigurationpb.Manager
	replicas           map[hotstuff.ID]consensus.Replica
	proposeCancel      context.CancelFunc
	timeoutCancel      context.CancelFunc
	reconfigCancel     context.CancelFunc
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (cfg *Config) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	cfg.mods = mods

	opts := *cfg.optsPtr
	cfg.optsPtr = nil // we don't need to keep the options around beyond this point, so we'll allow them to be GCed.

	// embed own ID to allow other replicas to identify messages from this replica
	md := metadata.New(map[string]string{
		"id": fmt.Sprintf("%d", cfg.mods.ID()),
	})

	opts = append(opts, gorums.WithMetadata(md))

	cfg.mgr = hotstuffpb.NewManager(opts...)

	cfg.readConfigMgr = hotstuffpb.NewManager(opts...)

	cfg.learnConfigMgr = hotstuffpb.NewManager(opts...)

	cfg.reconfigManager = reconfigurationpb.NewManager(opts...)
	cfg.mods.EventLoop().RegisterHandler(consensus.ReconfigurationStartEvent{}, func(event interface{}) {
		reconfigEvent := event.(consensus.ReconfigurationStartEvent)
		cfg.OnReconfigStartEvent(reconfigEvent)
	})
}

func (cfg *Config) OnReconfigStartEvent(event consensus.ReconfigurationStartEvent) {
	var ctx context.Context
	cfg.reconfigCancel()
	ctx, cfg.reconfigCancel = context.WithCancel(context.Background())
	req := &orchestrationpb.ReconfigurationRequest{
		Replicas: hotstuffpb.ReplicaStatusConvertToProto(event.Replicas)}
	cfg.activeConfig.ReconfigureStart(ctx, req)
}

// NewConfig creates a new configuration.
func NewConfig(creds credentials.TransportCredentials, opts ...gorums.ManagerOption) *Config {
	if creds == nil {
		creds = insecure.NewCredentials()
	}
	grpcOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithReturnConnectionError(),
		grpc.WithTransportCredentials(creds),
	}
	opts = append(opts, gorums.WithGrpcDialOptions(grpcOpts...))

	// initialization will be finished by InitConsensusModule
	cfg := &Config{
		replicas:      make(map[hotstuff.ID]consensus.Replica),
		optsPtr:       &opts,
		proposeCancel: func() {},
		timeoutCancel: func() {},
	}
	return cfg
}

// ReplicaInfo holds information about a replica.
type ReplicaInfo struct {
	ID        hotstuff.ID
	Address   string
	PubKey    consensus.PublicKey
	NodeState hotstuff.ReplicaState
}

// Connect opens connections to the replicas in the configuration.
// TODO(hanish): form configuration for the orchestrators and learn and read configurations
func (cfg *Config) Connect(replicas []ReplicaInfo) (err error) {
	// set up an ID mapping to give to gorums
	activeIDMapping := make(map[string]uint32)
	orchestratorsIDMapping := make(map[string]uint32)
	learnIDMapping := make(map[string]uint32)
	for _, replica := range replicas {
		// also initialize Replica structures
		cfg.replicas[replica.ID] = &Replica{
			id:            replica.ID,
			pubKey:        replica.PubKey,
			newviewCancel: func() {},
			voteCancel:    func() {},
			state:         replica.NodeState,
		}
		// we do not want to connect to ourself
		if replica.ID != cfg.mods.ID() {
			if replica.NodeState == hotstuff.Active {
				activeIDMapping[replica.Address] = uint32(replica.ID)
			} else if replica.NodeState == hotstuff.Orchestrator {
				orchestratorsIDMapping[replica.Address] = uint32(replica.ID)
			} else if replica.NodeState == hotstuff.Learn {
				learnIDMapping[replica.Address] = uint32(replica.ID)
			}
		}
	}

	// this will connect to the replicas
	cfg.activeConfig, err = cfg.mgr.NewConfiguration(qspec{}, gorums.WithNodeMap(activeIDMapping))
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	cfg.orchestratorConfig, err = cfg.reconfigManager.NewConfiguration(qspec{}, gorums.WithNodeMap(orchestratorsIDMapping))
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	cfg.learnConfig, err = cfg.learnConfigMgr.NewConfiguration(qspec{}, gorums.WithNodeMap(learnIDMapping))
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}
	// now we need to update the "node" field of each replica we connected to
	for _, node := range cfg.activeConfig.Nodes() {
		// the node ID should correspond with the replica ID
		// because we already configured an ID mapping for gorums to use.
		id := hotstuff.ID(node.ID())
		replica := cfg.replicas[id].(*Replica)
		replica.node = node
	}

	for _, node := range cfg.orchestratorConfig.Nodes() {
		id := hotstuff.ID(node.ID())
		replica := cfg.replicas[id].(*Replica)
		replica.orchestratorNode = node
	}
	// No need to assign learn as no unicast/multicast call is made to learn configuration
	return nil
}

// Replicas returns all of the replicas in the configuration.
func (cfg *Config) Replicas() map[hotstuff.ID]consensus.Replica {
	return cfg.replicas
}

// Replica returns a replica if it is present in the configuration.
func (cfg *Config) Replica(id hotstuff.ID) (replica consensus.Replica, ok bool) {
	replica, ok = cfg.replicas[id]
	return
}

// Len returns the number of replicas in the configuration.
func (cfg *Config) Len() int {
	return len(cfg.replicas)
}

// QuorumSize returns the size of a quorum
func (cfg *Config) QuorumSize() int {
	return hotstuff.QuorumSize(cfg.Len())
}

// Propose sends the block to all replicas in the configuration
func (cfg *Config) Propose(proposal consensus.ProposeMsg) {
	if cfg.activeConfig == nil {
		return
	}
	var ctx context.Context
	cfg.proposeCancel()
	ctx, cfg.proposeCancel = context.WithCancel(context.Background())
	p := hotstuffpb.ProposalToProto(proposal)
	cfg.activeConfig.Propose(ctx, p, gorums.WithNoSendWaiting())
}

// Timeout sends the timeout message to all replicas.
func (cfg *Config) Timeout(msg consensus.TimeoutMsg) {
	if cfg.activeConfig == nil {
		return
	}
	var ctx context.Context
	cfg.timeoutCancel()
	ctx, cfg.timeoutCancel = context.WithCancel(context.Background())
	cfg.activeConfig.Timeout(ctx, hotstuffpb.TimeoutMsgToProto(msg), gorums.WithNoSendWaiting())
}

// Fetch requests a block from all the replicas in the configuration
func (cfg *Config) Fetch(ctx context.Context, hash consensus.Hash) (*consensus.Block, bool) {
	protoBlock, err := cfg.activeConfig.Fetch(ctx, &hotstuffpb.BlockHash{Hash: hash[:]})
	if err != nil {
		qcErr, ok := err.(gorums.QuorumCallError)
		// filter out context errors
		if !ok || (qcErr.Reason != context.Canceled.Error() && qcErr.Reason != context.DeadlineExceeded.Error()) {
			cfg.mods.Logger().Infof("Failed to fetch block: %v", err)
		}
		return nil, false
	}
	return hotstuffpb.BlockFromProto(protoBlock), true
}

// Close closes all connections made by this configuration.
func (cfg *Config) Close() {
	cfg.mgr.Close()
}

func (cfg *Config) UpdateConfigurations(replicaInfo map[uint32]orchestrationpb.ReplicaState) error {
	activeIDMapping := make(map[string]uint32)
	orchestratorsIDMapping := make(map[string]uint32)
	learnIDMapping := make(map[string]uint32)
	for id, replicaState := range replicaInfo {
		nodeID := hotstuff.ID(id)
		replica := cfg.replicas[nodeID]
		replica.SetReplicaState(hotstuff.ReplicaState(replicaState))
		cfg.replicas[nodeID] = replica
		// we do not want to connect to ourself
		if replica.ID() != cfg.mods.ID() {
			if replica.IsActive() {
				activeIDMapping[replica.GetAddress()] = uint32(replica.ID())
			} else if replica.IsOrchestrator() {
				orchestratorsIDMapping[replica.GetAddress()] = uint32(replica.ID())
			} else if replica.IsLearn() {
				learnIDMapping[replica.GetAddress()] = uint32(replica.ID())
			}
		}
	}
	var err error
	// this will connect to the replicas
	cfg.activeConfig, err = cfg.mgr.NewConfiguration(qspec{}, gorums.WithNodeMap(activeIDMapping))
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	cfg.orchestratorConfig, err = cfg.reconfigManager.NewConfiguration(qspec{}, gorums.WithNodeMap(orchestratorsIDMapping))
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	cfg.learnConfig, err = cfg.learnConfigMgr.NewConfiguration(qspec{}, gorums.WithNodeMap(learnIDMapping))
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}
	return nil
}

var _ consensus.Configuration = (*Config)(nil)

type qspec struct {
	quorumSize int
	mods       *consensus.Modules
}

// FetchQF is the quorum function for the Fetch quorum call method.
// It simply returns true if one of the replies matches the requested block.
func (q qspec) FetchQF(in *hotstuffpb.BlockHash, replies map[uint32]*hotstuffpb.Block) (*hotstuffpb.Block, bool) {
	var h consensus.Hash
	copy(h[:], in.GetHash())
	for _, b := range replies {
		block := hotstuffpb.BlockFromProto(b)
		if h == block.Hash() {
			return b, true
		}
	}
	return nil, false
}

func (q qspec) ReconfigureStartQF(in *orchestrationpb.ReconfigurationRequest,
	replies map[uint32]*hotstuffpb.SyncInfo) (*hotstuffpb.SyncInfo, bool) {
	if len(replies) < q.quorumSize {
		return nil, false
	}
	var highestView uint64
	var highestTCView uint64
	var validQC int
	var highestQC *hotstuffpb.QuorumCert
	var highestTC *hotstuffpb.TimeoutCert
	for _, syncInfo := range replies {
		if q.mods.Crypto().VerifyQuorumCert(hotstuffpb.QuorumCertFromProto(syncInfo.QC)) {
			validQC++
		} else {
			continue
		}
		if syncInfo.TC != nil {
			if highestTCView < syncInfo.TC.GetView() {
				highestTC = syncInfo.TC
				highestTCView = syncInfo.TC.GetView()
			}
		}
		if syncInfo.QC.View > highestView {
			highestView = syncInfo.QC.View
			highestQC = syncInfo.QC
		}
	}
	ret := hotstuffpb.SyncInfo{}
	ret.QC = highestQC
	if highestTCView > highestView {
		ret.TC = highestTC
	}
	if validQC >= q.quorumSize {
		return &ret, true
	} else {
		return nil, false
	}
}

func (cfg *Config) ActiveReplicaForIndex(viewIndex int) hotstuff.ID {
	cfg.Lock()
	defer cfg.Unlock()
	idList := cfg.getSortedActiveIds()
	index := int(viewIndex) % len(idList)
	return idList[index]
}

func (cfg *Config) getSortedActiveIds() []hotstuff.ID {
	idList := make([]hotstuff.ID, 0)
	for id, replica := range cfg.replicas {
		if replica.IsActive() {
			idList = append(idList, id)
		}
	}
	sort.Slice(idList, func(i int, j int) bool {
		return idList[i] < idList[j]
	})
	return idList
}

func (cfg *Config) GetLowestActiveId() hotstuff.ID {
	cfg.Lock()
	defer cfg.Unlock()
	return cfg.getSortedActiveIds()[0]
}

func (cfg *Config) GetOrchestratorReplicas() map[hotstuff.ID]consensus.Replica {
	orchestrators := make(map[hotstuff.ID]consensus.Replica)
	for id, replica := range cfg.replicas {
		if replica.IsOrchestrator() {
			orchestrators[id] = replica
		}
	}
	return orchestrators
}

// GetLowestOrchestratorID returns the lowest ID of the orchestrators
func (cfg *Config) GetLowestOrchestratorID() hotstuff.ID {
	lowestID := hotstuff.ID(1)
	for id, replica := range cfg.replicas {
		if replica.IsOrchestrator() && lowestID <= replica.ID() {
			lowestID = id
		}
	}
	return lowestID
}

// OrchestratorForIndex returns the ID in the index of the sorted orchestrators list
func (cfg *Config) OrchestratorForIndex(viewIndex int) hotstuff.ID {
	orchestratorIDList := make([]hotstuff.ID, 0)
	for id, replica := range cfg.replicas {
		if replica.IsOrchestrator() {
			orchestratorIDList = append(orchestratorIDList, id)
		}
	}
	sort.Slice(orchestratorIDList, func(i int, j int) bool {
		return orchestratorIDList[i] < orchestratorIDList[j]
	})
	index := viewIndex % len(orchestratorIDList)
	return orchestratorIDList[index]
}
