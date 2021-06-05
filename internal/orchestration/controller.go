package orchestration

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"fmt"
	"net"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"google.golang.org/grpc"
)

// Experiment holds variables for an experiment.
type Experiment struct {
	NumReplicas    int
	NumClients     int
	BatchSize      int
	PayloadSize    int
	Duration       time.Duration
	Consensus      string
	Crypto         string
	LeaderRotation string

	mgr    *orchestrationpb.Manager
	config *orchestrationpb.Configuration

	// the replica IDs associated with each node.
	nodesToReplicas map[uint32][]consensus.ID
	caKey           *ecdsa.PrivateKey
	ca              *x509.Certificate
}

func (e *Experiment) Run(hosts []string) error {
	err := e.connect(hosts)
	if err != nil {
		return fmt.Errorf("failed to connect to hosts: %w", err)
	}

	cfg, err := e.createReplicas()
	if err != nil {
		return fmt.Errorf("failed to create replicas: %w", err)
	}

	err = e.startReplicas(cfg)
	if err != nil {
		return fmt.Errorf("failed to start replicas: %w", err)
	}

	time.Sleep(e.Duration)

	err = e.stopReplicas()
	if err != nil {
		return fmt.Errorf("failed to stop replicas: %w", err)
	}

	return nil
}

func (e *Experiment) connect(hosts []string) (err error) {
	e.mgr = orchestrationpb.NewManager(
		gorums.WithDialTimeout(10*time.Second),
		gorums.WithGrpcDialOptions(grpc.WithBlock(), grpc.WithInsecure()),
	)
	e.config, err = e.mgr.NewConfiguration(qspec{e}, gorums.WithNodeList(hosts))
	return err
}

func (e *Experiment) createReplicas() (cfg *orchestrationpb.ReplicaConfiguration, err error) {
	e.nodesToReplicas = make(map[uint32][]consensus.ID)

	e.caKey, e.ca, err = keygen.GenerateCA()
	if err != nil {
		return nil, err
	}

	nextID := consensus.ID(1)

	cfg, err = e.config.CreateReplica(
		context.Background(),
		&orchestrationpb.CreateReplicaRequest{Replicas: make(map[uint32]*orchestrationpb.ReplicaOpts)},
		func(crr *orchestrationpb.CreateReplicaRequest, u uint32) *orchestrationpb.CreateReplicaRequest {
			for i := 0; i < e.NumReplicas/e.config.Size(); i++ {
				opts := orchestrationpb.ReplicaOpts{
					CertificateAuthority: keygen.CertToPEM(e.ca),
					UseTLS:               true,
					Crypto:               e.Crypto,
					Consensus:            e.Consensus,
					LeaderElection:       e.LeaderRotation,
					BatchSize:            uint32(e.BatchSize),
					BlockCacheSize:       uint32(5 * e.NumReplicas),
					InitialTimeout:       1000,
					TimeoutSamples:       1000,
					TimeoutMultiplier:    1000,
				}
				id := nextID
				node, _ := e.mgr.Node(u)
				keyChain, err := keygen.GenerateKeyChain(id, node.Host(), e.Crypto, e.ca, e.caKey)
				if err != nil {
					panic("failed to generate keychain")
				}
				e.nodesToReplicas[u] = append(e.nodesToReplicas[u], id)
				nextID++
				opts.ID = uint32(id)
				opts.PrivateKey = keyChain.PrivateKey
				opts.PublicKey = keyChain.PublicKey
				opts.Certificate = keyChain.Certificate
				opts.CertificateKey = keyChain.CertificateKey
				crr.Replicas[uint32(id)] = &opts
			}
			return crr
		},
	)
	return
}

func (e *Experiment) startReplicas(cfg *orchestrationpb.ReplicaConfiguration) error {
	_, err := e.config.StartReplica(context.Background(), &orchestrationpb.StartReplicaRequest{
		Configuration: cfg.GetReplicas(),
	}, func(srr *orchestrationpb.StartReplicaRequest, u uint32) *orchestrationpb.StartReplicaRequest {
		srr.IDs = e.getReplicaIDs(u)
		return srr
	})
	return err
}

func (e *Experiment) stopReplicas() error {
	_, err := e.config.StopReplica(context.Background(), &orchestrationpb.StopReplicaRequest{},
		func(srr *orchestrationpb.StopReplicaRequest, u uint32) *orchestrationpb.StopReplicaRequest {
			srr.IDs = e.getReplicaIDs(u)
			return srr
		},
	)
	return err
}

func (e *Experiment) getReplicaIDs(nodeID uint32) []uint32 {
	var ids []uint32
	for _, id := range e.nodesToReplicas[nodeID] {
		ids = append(ids, uint32(id))
	}
	return ids
}

type qspec struct {
	e *Experiment
}

func (q qspec) CreateReplicaQF(_ *orchestrationpb.CreateReplicaRequest, replies map[uint32]*orchestrationpb.CreateReplicaResponse) (*orchestrationpb.ReplicaConfiguration, bool) {
	if len(replies) != q.e.config.Size() {
		return nil, false
	}
	cfg := make(map[uint32]*orchestrationpb.ReplicaInfo)

	for nodeID, reply := range replies {
		node, ok := q.e.mgr.Node(nodeID)
		if !ok {
			panic("reply from node that does not exist in manager")
		}
		for _, replica := range reply.GetReplicas() {
			host, _, err := net.SplitHostPort(node.Address())
			if err != nil {
				panic(fmt.Errorf("invalid node address: %w", err))
			}
			replica.Address = host
			cfg[replica.ID] = replica
		}
	}
	return &orchestrationpb.ReplicaConfiguration{
		Replicas: cfg,
	}, true
}

func (q qspec) StartReplicaQF(_ *orchestrationpb.StartReplicaRequest, replies map[uint32]*orchestrationpb.StartReplicaResponse) (*orchestrationpb.StartReplicaResponse, bool) {
	return &orchestrationpb.StartReplicaResponse{}, len(replies) == q.e.config.Size()
}

func (q qspec) StopReplicaQF(_ *orchestrationpb.StopReplicaRequest, replies map[uint32]*orchestrationpb.StopReplicaResponse) (*orchestrationpb.StopReplicaResponse, bool) {
	return &orchestrationpb.StopReplicaResponse{}, len(replies) == q.e.config.Size()
}

func (q qspec) CreateClientQF(_ *orchestrationpb.CreateClientRequest, replies map[uint32]*orchestrationpb.CreateClientResponse) (*orchestrationpb.CreateClientResponse, bool) {
	return &orchestrationpb.CreateClientResponse{}, len(replies) == q.e.config.Size()
}

func (q qspec) StartClientQF(_ *orchestrationpb.StartClientRequest, replies map[uint32]*orchestrationpb.StartClientResponse) (*orchestrationpb.StartClientResponse, bool) {
	return &orchestrationpb.StartClientResponse{}, len(replies) == q.e.config.Size()
}

func (q qspec) StopClientQF(_ *orchestrationpb.StopClientRequest, replies map[uint32]*orchestrationpb.StopClientResponse) (*orchestrationpb.StopClientResponse, bool) {
	return &orchestrationpb.StopClientResponse{}, len(replies) == q.e.config.Size()
}
