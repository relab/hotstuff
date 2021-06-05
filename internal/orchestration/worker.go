package orchestration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/replica"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Worker runs clients and replicas.
type Worker struct {
	replicas map[consensus.ID]*replica.Replica
	clients  map[consensus.ID]*client.Client
}

// NewWorker returns a new worker.
func NewWorker() *Worker {
	return &Worker{
		replicas: make(map[consensus.ID]*replica.Replica),
		clients:  make(map[consensus.ID]*client.Client),
	}
}

func (w *Worker) CreateReplica(ctx context.Context, req *orchestrationpb.CreateReplicaRequest, ret func(*orchestrationpb.CreateReplicaResponse, error)) {
	resp := &orchestrationpb.CreateReplicaResponse{Replicas: make(map[uint32]*orchestrationpb.ReplicaInfo)}
	for _, cfg := range req.GetReplicas() {
		privKey, err := keygen.ParsePrivateKey(cfg.GetPrivateKey())
		if err != nil {
			ret(nil, err)
			return
		}
		var certificate tls.Certificate
		var rootCAs *x509.CertPool
		if cfg.GetUseTLS() {
			certificate, err = tls.X509KeyPair(cfg.GetCertificate(), cfg.GetCertificateKey())
			if err != nil {
				ret(nil, err)
				return
			}
			rootCAs = x509.NewCertPool()
			rootCAs.AppendCertsFromPEM(cfg.GetCertificateAuthority())
		}
		c := replica.Config{
			ID:                consensus.ID(cfg.GetID()),
			PrivateKey:        privKey,
			TLS:               cfg.GetUseTLS(),
			Certificate:       &certificate,
			RootCAs:           rootCAs,
			Consensus:         cfg.GetConsensus(),
			Crypto:            cfg.GetCrypto(),
			LeaderRotation:    cfg.GetLeaderElection(),
			BatchSize:         cfg.GetBatchSize(),
			BlockCacheSize:    cfg.GetBlockCacheSize(),
			InitialTimeout:    float64(cfg.GetInitialTimeout()),
			TimeoutSamples:    cfg.GetTimeoutSamples(),
			TimeoutMultiplier: float64(cfg.GetTimeoutMultiplier()),
			Output:            nopCloser{io.Discard},
			ManagerOptions: []gorums.ManagerOption{
				gorums.WithDialTimeout(10 * time.Second), // TODO: make this configurable?
			},
		}
		r, err := replica.New(c)
		if err != nil {
			ret(nil, err)
			return
		}

		replicaListener, err := net.Listen("tcp", ":0")
		if err != nil {
			ret(nil, err)
			return
		}
		replicaPort, err := getPort(replicaListener)
		if err != nil {
			ret(nil, err)
			return
		}
		clientListener, err := net.Listen("tcp", ":0")
		if err != nil {
			ret(nil, err)
			return
		}
		clientPort, err := getPort(clientListener)
		if err != nil {
			ret(nil, err)
			return
		}

		r.StartServers(replicaListener, clientListener)
		w.replicas[c.ID] = r

		resp.Replicas[cfg.GetID()] = &orchestrationpb.ReplicaInfo{
			ID:          cfg.GetID(),
			PublicKey:   cfg.GetPublicKey(),
			ReplicaPort: replicaPort,
			ClientPort:  clientPort,
		}
	}
	ret(resp, nil)
}

func (w *Worker) StartReplica(_ context.Context, req *orchestrationpb.StartReplicaRequest, ret func(*orchestrationpb.StartReplicaResponse, error)) {
	for _, id := range req.GetIDs() {
		replica, ok := w.replicas[consensus.ID(id)]
		if !ok {
			ret(nil, status.Errorf(codes.NotFound, "The replica with ID %d was not found.", id))
			return
		}
		cfg, err := getConfiguration(consensus.ID(id), req.GetConfiguration())
		if err != nil {
			ret(nil, err)
			return
		}
		err = replica.Connect(cfg)
		if err != nil {
			ret(nil, err)
		}
		replica.Start()
	}
	ret(&orchestrationpb.StartReplicaResponse{}, nil)
}

func (w *Worker) StopReplica(_ context.Context, req *orchestrationpb.StopReplicaRequest, ret func(*orchestrationpb.StopReplicaResponse, error)) {
	for _, id := range req.GetIDs() {
		r, ok := w.replicas[consensus.ID(id)]
		if !ok {
			ret(nil, status.Errorf(codes.NotFound, "The replica with id %d was not found.", id))
			return
		}
		r.Stop()
		// TODO: return test results
	}
	ret(&orchestrationpb.StopReplicaResponse{}, nil)
}

func (w *Worker) CreateClient(_ context.Context, _ *orchestrationpb.CreateClientRequest, _ func(*orchestrationpb.CreateClientResponse, error)) {
	panic("not implemented") // TODO: Implement
}

func (w *Worker) StartClient(_ context.Context, _ *orchestrationpb.StartClientRequest, _ func(*orchestrationpb.StartClientResponse, error)) {
	panic("not implemented") // TODO: Implement
}

func (w *Worker) StopClient(_ context.Context, _ *orchestrationpb.StopClientRequest, _ func(*orchestrationpb.StopClientResponse, error)) {
	panic("not implemented") // TODO: Implement
}

func getCertificate(conf *orchestrationpb.ReplicaOpts) (*tls.Certificate, error) {
	if conf.GetUseTLS() && conf.GetCertificate() != nil {
		var key []byte
		if conf.GetCertificateKey() != nil {
			key = conf.GetCertificateKey()
		} else {
			key = conf.GetPrivateKey()
		}
		cert, err := tls.X509KeyPair(conf.GetCertificate(), key)
		if err != nil {
			return nil, err
		}
		return &cert, nil
	}
	return nil, nil
}

func getConfiguration(id consensus.ID, conf map[uint32]*orchestrationpb.ReplicaInfo) (*config.ReplicaConfig, error) {
	cfg := &config.ReplicaConfig{ID: id, Replicas: make(map[consensus.ID]*config.ReplicaInfo)}

	for _, replica := range conf {
		pubKey, err := keygen.ParsePublicKey(replica.GetPublicKey())
		if err != nil {
			return nil, err
		}
		cfg.Replicas[consensus.ID(replica.GetID())] = &config.ReplicaInfo{
			ID:      consensus.ID(replica.GetID()),
			Address: net.JoinHostPort(replica.GetAddress(), strconv.Itoa(int(replica.GetReplicaPort()))),
			PubKey:  pubKey,
		}
	}
	return cfg, nil
}

func getPort(lis net.Listener) (uint32, error) {
	_, portStr, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		return 0, err
	}
	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(port), nil
}

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error { return nil }
