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
	"github.com/relab/hotstuff/client"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/globals"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/dependencies"
	"github.com/relab/hotstuff/internal/latency"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/metrics"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/replica"
	"github.com/relab/hotstuff/security/crypto/keygen"
	"github.com/relab/hotstuff/service/cmdcache"
	"github.com/relab/hotstuff/service/server"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	// imported modules
	_ "github.com/relab/hotstuff/protocol/kauri"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	_ "github.com/relab/hotstuff/protocol/leaderrotation"
	_ "github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	_ "github.com/relab/hotstuff/protocol/rules/fasthotstuff"
	_ "github.com/relab/hotstuff/protocol/rules/simplehotstuff"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	_ "github.com/relab/hotstuff/security/crypto/bls12"
	_ "github.com/relab/hotstuff/security/crypto/ecdsa"
	_ "github.com/relab/hotstuff/security/crypto/eddsa"
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
		clientListener, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, fmt.Errorf("failed to create listener: %w", err)
		}
		r.StartServers(replicaListener, clientListener)
		w.replicas[hotstuff.ID(cfg.GetID())] = r

		resp.Replicas[cfg.GetID()] = &orchestrationpb.ReplicaInfo{
			ID:          cfg.GetID(),
			PublicKey:   cfg.GetPublicKey(),
			ReplicaPort: uint32(replicaListener.Addr().(*net.TCPAddr).Port),
			ClientPort:  uint32(clientListener.Addr().(*net.TCPAddr).Port),
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
	// setup core - used in replica and measurement framework
	globalOpts := []globals.GlobalOption{}
	if opts.GetLeaderRotation() == leaderrotation.TreeLeaderModuleName {
		delayMode := tree.DelayTypeNone
		if opts.GetAggregationTime() {
			delayMode = tree.DelayTypeAggregation
		}
		t := tree.NewDelayed(
			opts.HotstuffID(),
			delayMode,
			int(opts.GetBranchFactor()),
			latency.MatrixFrom(opts.GetLocations()),
			opts.TreePositionIDs(),
			opts.GetTreeDelta().AsDuration(),
		)
		globalOpts = append(globalOpts, globals.WithTree(t))
	}
	if opts.GetKauri() {
		globalOpts = append(globalOpts, globals.WithKauri())
	}
	globalOpts = append(globalOpts, globals.WithSharedRandomSeed(opts.GetSharedSeed()))
	depsCore := dependencies.NewCore(opts.HotstuffID(), "hs", privKey, globalOpts...)
	// check if measurements should be enabled
	if w.measurementInterval > 0 {
		// Initializes the metrics modules internally.
		err = metrics.Enable(
			depsCore.EventLoop(),
			depsCore.Logger(),
			w.metricsLogger,
			depsCore.Globals(),
			w.measurementInterval,
			w.metrics...,
		)
		if err != nil {
			return nil, err
		}
	}
	// setup replica
	replicaOpts := []replica.Option{}
	var certificate tls.Certificate
	var rootCAs *x509.CertPool
	if opts.GetUseTLS() {
		certificate, err = tls.X509KeyPair(opts.GetCertificate(), opts.GetCertificateKey())
		if err != nil {
			return nil, err
		}
		rootCAs = x509.NewCertPool()
		rootCAs.AppendCertsFromPEM(opts.GetCertificateAuthority())
		replicaOpts = append(replicaOpts, replica.WithTLS(certificate, rootCAs))
	}
	replicaOpts = append(replicaOpts,
		replica.OverrideCrypto(opts.GetCrypto()),
		replica.OverrideConsensusRules(opts.GetConsensus()),
		replica.OverrideLeaderRotation(opts.GetLeaderRotation()),
		replica.WithByzantineStrategy(opts.GetByzantineStrategy()),
		replica.WithCmdCacheOptions(cmdcache.WithBatching(opts.GetBatchSize())),
		replica.WithServerOptions(server.WithLatencies(opts.HotstuffID(), opts.GetLocations())),
	)
	return replica.New(
		depsCore,
		viewduration.NewParams(
			opts.GetTimeoutSamples(),
			opts.GetInitialTimeout().AsDuration(),
			opts.GetMaxTimeout().AsDuration(),
			opts.GetTimeoutMultiplier(),
		),
		replicaOpts...,
	)
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
			},
			RateLimit:        opts.GetRateLimit(),
			RateStep:         opts.GetRateStep(),
			RateStepInterval: opts.GetRateStepInterval().AsDuration(),
			Timeout:          opts.GetTimeout().AsDuration(),
		}
		buildOpt := globals.NewGlobals(hotstuff.ID(opts.GetID()), nil)
		logger := logging.New("cli" + strconv.Itoa(int(opts.GetID())))
		eventLoop := eventloop.New(logger, 1000)

		if w.measurementInterval > 0 {
			err := metrics.Enable(
				eventLoop,
				logger,
				w.metricsLogger,
				buildOpt,
				w.measurementInterval,
				w.metrics...,
			)
			if err != nil {
				return nil, err
			}
		}

		cli := client.New(
			eventLoop,
			logger,
			buildOpt,
			c,
		)
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

func getConfiguration(conf map[uint32]*orchestrationpb.ReplicaInfo, client bool) ([]hotstuff.ReplicaInfo, error) {
	replicas := make([]hotstuff.ReplicaInfo, 0, len(conf))
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
		replicas = append(replicas, hotstuff.ReplicaInfo{
			ID:      hotstuff.ID(replica.GetID()),
			Address: addr,
			PubKey:  pubKey,
		})
	}
	return replicas, nil
}
