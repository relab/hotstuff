package orchestration

import (
	"crypto/rand"
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Worker runs clients and replicas.
type Worker struct {
	replicas     map[consensus.ID]*replica.Replica
	clients      map[consensus.ID]*client.Client
	quitCallback func()
}

// NewWorker returns a new worker.
func NewWorker(quitCallback func()) *Worker {
	return &Worker{
		replicas:     make(map[consensus.ID]*replica.Replica),
		clients:      make(map[consensus.ID]*client.Client),
		quitCallback: quitCallback,
	}
}

func (w *Worker) CreateReplica(ctx gorums.ServerCtx, req *orchestrationpb.CreateReplicaRequest) (*orchestrationpb.CreateReplicaResponse, error) {
	resp := &orchestrationpb.CreateReplicaResponse{Replicas: make(map[uint32]*orchestrationpb.ReplicaInfo)}
	for _, cfg := range req.GetReplicas() {
		privKey, err := keygen.ParsePrivateKey(cfg.GetPrivateKey())
		if err != nil {
			return nil, err
		}
		var certificate tls.Certificate
		var rootCAs *x509.CertPool
		if cfg.GetUseTLS() {
			certificate, err = tls.X509KeyPair(cfg.GetCertificate(), cfg.GetCertificateKey())
			if err != nil {
				return nil, err
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
			Output:            writeNopCloser{io.Discard},
			ManagerOptions: []gorums.ManagerOption{
				gorums.WithDialTimeout(time.Duration(cfg.GetConnectTimeout() * float32(time.Millisecond))),
				gorums.WithGrpcDialOptions(grpc.WithReturnConnectionError()),
			},
		}
		r, err := replica.New(c)
		if err != nil {
			return nil, err
		}

		replicaListener, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, err
		}
		replicaPort, err := getPort(replicaListener)
		if err != nil {
			return nil, err
		}
		clientListener, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, err
		}
		clientPort, err := getPort(clientListener)
		if err != nil {
			return nil, err
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
	return resp, nil
}

func (w *Worker) StartReplica(_ gorums.ServerCtx, req *orchestrationpb.StartReplicaRequest) (*orchestrationpb.StartReplicaResponse, error) {
	for _, id := range req.GetIDs() {
		replica, ok := w.replicas[consensus.ID(id)]
		if !ok {
			return nil, status.Errorf(codes.NotFound, "The replica with ID %d was not found.", id)
		}
		cfg, err := getConfiguration(consensus.ID(id), req.GetConfiguration(), false)
		if err != nil {
			return nil, err
		}
		err = replica.Connect(cfg)
		if err != nil {
			return nil, err
		}
		defer replica.Start()
	}
	return &orchestrationpb.StartReplicaResponse{}, nil
}

func (w *Worker) StopReplica(_ gorums.ServerCtx, req *orchestrationpb.StopReplicaRequest) (*orchestrationpb.StopReplicaResponse, error) {
	res := &orchestrationpb.StopReplicaResponse{
		Hashes: make(map[uint32][]byte),
	}
	for _, id := range req.GetIDs() {
		r, ok := w.replicas[consensus.ID(id)]
		if !ok {
			return nil, status.Errorf(codes.NotFound, "The replica with id %d was not found.", id)
		}
		r.Stop()
		res.Hashes[id] = r.GetHash()
		// TODO: return test results
	}
	return res, nil
}

func (w *Worker) StartClient(_ gorums.ServerCtx, req *orchestrationpb.StartClientRequest) (*orchestrationpb.StartClientResponse, error) {
	ca := req.GetCertificateAuthority()
	cp := x509.NewCertPool()
	cp.AppendCertsFromPEM(ca)
	for _, opts := range req.GetClients() {
		c := client.Config{
			ID:            consensus.ID(opts.GetID()),
			TLS:           opts.GetUseTLS(),
			RootCAs:       cp,
			MaxConcurrent: opts.GetMaxConcurrent(),
			PayloadSize:   opts.GetPayloadSize(),
			Input:         readNopCloser{rand.Reader},
			ManagerOptions: []gorums.ManagerOption{
				gorums.WithDialTimeout(time.Duration(opts.GetConnectTimeout() * float32(time.Millisecond))),
				gorums.WithGrpcDialOptions(grpc.WithReturnConnectionError()),
			},
		}
		cli := client.New(c)
		cfg, err := getConfiguration(consensus.ID(opts.GetID()), req.GetConfiguration(), true)
		if err != nil {
			return nil, err
		}
		err = cli.Connect(cfg)
		if err != nil {
			return nil, err
		}
		cli.Start()
		w.clients[consensus.ID(opts.GetID())] = cli
	}
	return &orchestrationpb.StartClientResponse{}, nil
}

func (w *Worker) StopClient(_ gorums.ServerCtx, req *orchestrationpb.StopClientRequest) (*orchestrationpb.StopClientResponse, error) {
	for _, id := range req.GetIDs() {
		cli, ok := w.clients[consensus.ID(id)]
		if !ok {
			return nil, status.Errorf(codes.NotFound, "the client with ID %d was not found", id)
		}
		cli.Stop()
	}
	return &orchestrationpb.StopClientResponse{}, nil
}

func (w *Worker) Quit(_ gorums.ServerCtx, req *emptypb.Empty) {
	w.quitCallback()
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

func getConfiguration(id consensus.ID, conf map[uint32]*orchestrationpb.ReplicaInfo, client bool) (*config.ReplicaConfig, error) {
	cfg := &config.ReplicaConfig{ID: id, Replicas: make(map[consensus.ID]*config.ReplicaInfo)}

	for _, replica := range conf {
		pubKey, err := keygen.ParsePublicKey(replica.GetPublicKey())
		if err != nil {
			return nil, err
		}
		var addr string
		if client {
			addr = net.JoinHostPort(replica.GetAddress(), strconv.Itoa(int(replica.GetClientPort())))
		} else {
			addr = net.JoinHostPort(replica.GetAddress(), strconv.Itoa(int(replica.GetReplicaPort())))
		}
		cfg.Replicas[consensus.ID(replica.GetID())] = &config.ReplicaInfo{
			ID:      consensus.ID(replica.GetID()),
			Address: addr,
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

type writeNopCloser struct {
	io.Writer
}

func (writeNopCloser) Close() error { return nil }

type readNopCloser struct {
	io.Reader
}

func (readNopCloser) Close() error { return nil }
