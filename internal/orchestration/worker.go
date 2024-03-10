package orchestration

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/consensus/byzantine"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/replica"
	"github.com/relab/hotstuff/synchronizer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	// imported modules
	_ "github.com/relab/hotstuff/consensus/chainedhotstuff"
	_ "github.com/relab/hotstuff/consensus/fasthotstuff"
	_ "github.com/relab/hotstuff/consensus/simplehotstuff"
	_ "github.com/relab/hotstuff/crypto/bls12"
	_ "github.com/relab/hotstuff/crypto/ecdsa"
	_ "github.com/relab/hotstuff/crypto/eddsa"
	_ "github.com/relab/hotstuff/handel"
	_ "github.com/relab/hotstuff/leaderrotation"
)

// Worker starts and runs clients and replicas based on commands from the controller.
type Worker struct {
	send *protostream.Writer
	recv *protostream.Reader

	metricsLogger       metrics.Logger
	metrics             []string
	measurementInterval time.Duration

	replicas map[hotstuff.ID]*replica.Replica
	clients  map[hotstuff.ID]*client.Client
}

// Run runs the worker until it receives a command to quit.
func (w *Worker) Run() error {
	for {
		msg, err := w.recv.ReadAny()
		if err != nil {
			return err
		}

		var res proto.Message
		switch req := msg.(type) {
		case *orchestrationpb.CreateReplicaRequest:
			res, err = w.createReplicas(req)
		case *orchestrationpb.StartReplicaRequest:
			res, err = w.startReplicas(req)
		case *orchestrationpb.StopReplicaRequest:
			res, err = w.stopReplicas(req)
		case *orchestrationpb.StartClientRequest:
			res, err = w.startClients(req)
		case *orchestrationpb.StopClientRequest:
			res, err = w.stopClients(req)
		case *orchestrationpb.QuitRequest:
			return nil
		}

		if err != nil {
			s, _ := status.FromError(err)
			res = s.Proto()
		}

		err = w.send.WriteAny(res)
		if err != nil {
			return err
		}
	}
}

// NewWorker returns a new worker.
func NewWorker(send *protostream.Writer, recv *protostream.Reader, dl metrics.Logger, metrics []string, measurementInterval time.Duration) Worker {
	return Worker{
		send:                send,
		recv:                recv,
		metricsLogger:       dl,
		metrics:             metrics,
		measurementInterval: measurementInterval,
		replicas:            make(map[hotstuff.ID]*replica.Replica),
		clients:             make(map[hotstuff.ID]*client.Client),
	}
}

func (w *Worker) createReplicas(req *orchestrationpb.CreateReplicaRequest) (*orchestrationpb.CreateReplicaResponse, error) {
	resp := &orchestrationpb.CreateReplicaResponse{Replicas: make(map[uint32]*orchestrationpb.ReplicaInfo)}
	for _, cfg := range req.GetReplicas() {
		r, err := w.createReplica(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create replica: %w", err)
		}

		// set up listeners and get the ports
		replicaListener, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, fmt.Errorf("failed to create listener: %w", err)
		}
		replicaPort, err := getPort(replicaListener)
		if err != nil {
			return nil, err
		}
		clientListener, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, fmt.Errorf("failed to create listener: %w", err)
		}
		clientPort, err := getPort(clientListener)
		if err != nil {
			return nil, err
		}

		r.StartServers(replicaListener, clientListener)
		w.replicas[hotstuff.ID(cfg.GetID())] = r

		resp.Replicas[cfg.GetID()] = &orchestrationpb.ReplicaInfo{
			ID:          cfg.GetID(),
			PublicKey:   cfg.GetPublicKey(),
			ReplicaPort: replicaPort,
			ClientPort:  clientPort,
		}
	}
	return resp, nil
}

func (w *Worker) createReplica(opts *orchestrationpb.ReplicaOpts) (*replica.Replica, error) {
	w.metricsLogger.Log(opts)

	// get private key and certificates
	privKey, err := keygen.ParsePrivateKey(opts.GetPrivateKey())
	if err != nil {
		return nil, err
	}
	var certificate tls.Certificate
	var rootCAs *x509.CertPool
	if opts.GetUseTLS() {
		certificate, err = tls.X509KeyPair(opts.GetCertificate(), opts.GetCertificateKey())
		if err != nil {
			return nil, err
		}
		rootCAs = x509.NewCertPool()
		rootCAs.AppendCertsFromPEM(opts.GetCertificateAuthority())
	}
	// prepare modules
	builder := modules.NewBuilder(hotstuff.ID(opts.GetID()), privKey)

	consensusRules, ok := modules.GetModule[consensus.Rules](opts.GetConsensus())
	if !ok {
		return nil, fmt.Errorf("invalid consensus name: '%s'", opts.GetConsensus())
	}

	if opts.GetByzantineStrategy() != "" {
		if byz, ok := modules.GetModule[byzantine.Byzantine](opts.GetByzantineStrategy()); ok {
			consensusRules = byz.Wrap(consensusRules)
		} else {
			return nil, fmt.Errorf("invalid byzantine strategy: '%s'", opts.GetByzantineStrategy())
		}
	}

	cryptoImpl, ok := modules.GetModule[modules.CryptoBase](opts.GetCrypto())
	if !ok {
		return nil, fmt.Errorf("invalid crypto name: '%s'", opts.GetCrypto())
	}

	leaderRotation, ok := modules.GetModule[modules.LeaderRotation](opts.GetLeaderRotation())
	if !ok {
		return nil, fmt.Errorf("invalid leader-rotation algorithm: '%s'", opts.GetLeaderRotation())
	}

	sync := synchronizer.New(synchronizer.NewViewDuration(
		uint64(opts.GetTimeoutSamples()),
		float64(opts.GetInitialTimeout().AsDuration().Nanoseconds())/float64(time.Millisecond),
		float64(opts.GetMaxTimeout().AsDuration().Nanoseconds())/float64(time.Millisecond),
		float64(opts.GetTimeoutMultiplier()),
	))
	builder.Add(
		eventloop.New(1000),
		consensus.New(consensusRules),
		consensus.NewVotingMachine(),
		crypto.NewCache(cryptoImpl, 100), // TODO: consider making this configurable
		leaderRotation,
		sync,
		w.metricsLogger,
		blockchain.New(),
		logging.New("hs"+strconv.Itoa(int(opts.GetID()))),
	)
	builder.Options().SetSharedRandomSeed(opts.GetSharedSeed())
	if w.measurementInterval > 0 {
		replicaMetrics := metrics.GetReplicaMetrics(w.metrics...)
		builder.Add(replicaMetrics...)
		builder.Add(metrics.NewTicker(w.measurementInterval))
	}

	for _, n := range opts.GetModules() {
		m, ok := modules.GetModuleUntyped(n)
		if !ok {
			return nil, fmt.Errorf("no module named '%s'", n)
		}
		builder.Add(m)
	}
	// convert location info map key from uint32 to hotstuff.ID
	locationInfo := make(map[hotstuff.ID]string)
	for k, v := range opts.GetLocationInfo() {
		locationInfo[hotstuff.ID(k)] = v
	}
	c := replica.Config{
		ID:           hotstuff.ID(opts.GetID()),
		PrivateKey:   privKey,
		TLS:          opts.GetUseTLS(),
		Certificate:  &certificate,
		RootCAs:      rootCAs,
		LocationInfo: locationInfo,
		BatchSize:    opts.GetBatchSize(),
		ManagerOptions: []gorums.ManagerOption{
			gorums.WithDialTimeout(opts.GetConnectTimeout().AsDuration()),
			gorums.WithGrpcDialOptions(grpc.WithReturnConnectionError()),
		},
	}
	return replica.New(c, builder), nil
}

func (w *Worker) startReplicas(req *orchestrationpb.StartReplicaRequest) (*orchestrationpb.StartReplicaResponse, error) {
	for _, id := range req.GetIDs() {
		replica, ok := w.replicas[hotstuff.ID(id)]
		if !ok {
			return nil, status.Errorf(codes.NotFound, "The replica with ID %d was not found.", id)
		}
		cfg, err := getConfiguration(req.GetConfiguration(), false)
		if err != nil {
			return nil, err
		}
		err = replica.Connect(cfg)
		if err != nil {
			return nil, err
		}

		defer func(id uint32) {
			w.metricsLogger.Log(&types.StartEvent{Event: types.NewReplicaEvent(id, time.Now())})
			replica.Start()
		}(id)
	}
	return &orchestrationpb.StartReplicaResponse{}, nil
}

func (w *Worker) stopReplicas(req *orchestrationpb.StopReplicaRequest) (*orchestrationpb.StopReplicaResponse, error) {
	res := &orchestrationpb.StopReplicaResponse{
		Hashes: make(map[uint32][]byte),
		Counts: make(map[uint32]uint32),
	}
	for _, id := range req.GetIDs() {
		r, ok := w.replicas[hotstuff.ID(id)]
		if !ok {
			return nil, status.Errorf(codes.NotFound, "The replica with id %d was not found.", id)
		}
		r.Stop()
		res.Hashes[id] = r.GetHash()
		res.Counts[id] = r.GetCmdCount()
		// TODO: return test results
	}
	return res, nil
}

func (w *Worker) startClients(req *orchestrationpb.StartClientRequest) (*orchestrationpb.StartClientResponse, error) {
	ca := req.GetCertificateAuthority()
	cp := x509.NewCertPool()
	cp.AppendCertsFromPEM(ca)
	for _, opts := range req.GetClients() {
		w.metricsLogger.Log(opts)

		c := client.Config{
			TLS:           opts.GetUseTLS(),
			RootCAs:       cp,
			MaxConcurrent: opts.GetMaxConcurrent(),
			PayloadSize:   opts.GetPayloadSize(),
			Input:         io.NopCloser(rand.Reader),
			ManagerOptions: []gorums.ManagerOption{
				gorums.WithDialTimeout(opts.GetConnectTimeout().AsDuration()),
				gorums.WithGrpcDialOptions(grpc.WithReturnConnectionError()),
			},
			RateLimit:        opts.GetRateLimit(),
			RateStep:         opts.GetRateStep(),
			RateStepInterval: opts.GetRateStepInterval().AsDuration(),
			Timeout:          opts.GetTimeout().AsDuration(),
		}
		mods := modules.NewBuilder(hotstuff.ID(opts.GetID()), nil)
		mods.Add(eventloop.New(1000))

		if w.measurementInterval > 0 {
			clientMetrics := metrics.GetClientMetrics(w.metrics...)
			mods.Add(clientMetrics...)
			mods.Add(metrics.NewTicker(w.measurementInterval))
		}

		mods.Add(w.metricsLogger)
		mods.Add(logging.New("cli" + strconv.Itoa(int(opts.GetID()))))
		cli := client.New(c, mods)
		cfg, err := getConfiguration(req.GetConfiguration(), true)
		if err != nil {
			return nil, err
		}
		err = cli.Connect(cfg)
		if err != nil {
			return nil, err
		}
		cli.Start()
		w.metricsLogger.Log(&types.StartEvent{Event: types.NewClientEvent(opts.GetID(), time.Now())})
		w.clients[hotstuff.ID(opts.GetID())] = cli
	}
	return &orchestrationpb.StartClientResponse{}, nil
}

func (w *Worker) stopClients(req *orchestrationpb.StopClientRequest) (*orchestrationpb.StopClientResponse, error) {
	for _, id := range req.GetIDs() {
		cli, ok := w.clients[hotstuff.ID(id)]
		if !ok {
			return nil, status.Errorf(codes.NotFound, "the client with ID %d was not found", id)
		}
		cli.Stop()
	}
	return &orchestrationpb.StopClientResponse{}, nil
}

func getConfiguration(conf map[uint32]*orchestrationpb.ReplicaInfo, client bool) ([]backend.ReplicaInfo, error) {
	replicas := make([]backend.ReplicaInfo, 0, len(conf))
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
		replicas = append(replicas, backend.ReplicaInfo{
			ID:      hotstuff.ID(replica.GetID()),
			Address: addr,
			PubKey:  pubKey,
		})
	}
	return replicas, nil
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
