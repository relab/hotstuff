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
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/latency"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/metrics"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/replica"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/keygen"
	"github.com/relab/hotstuff/server"
	"github.com/relab/hotstuff/wiring"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	// imported modules
	"github.com/relab/hotstuff/protocol/dissagg/clique"
	"github.com/relab/hotstuff/protocol/dissagg/kauri"
	_ "github.com/relab/hotstuff/protocol/leaderrotation"
	_ "github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	_ "github.com/relab/hotstuff/protocol/rules/fasthotstuff"
	_ "github.com/relab/hotstuff/protocol/rules/simplehotstuff"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/protocol/votingmachine"
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
	runtimeOpts := []core.RuntimeOption{}
	// TODO(AlanRostem): maybe rename the tree option to kauriTree? should also use the tree check only for enable kauri
	if opts.TreeEnabled() {
		runtimeOpts = append(runtimeOpts, core.WithKauriTree(newTree(opts)))
	}
	runtimeOpts = append(runtimeOpts, core.WithSharedRandomSeed(opts.GetSharedSeed()))
	if opts.GetUseAggQC() {
		runtimeOpts = append(runtimeOpts, core.WithAggregateQC())
	}
	depsCore := wiring.NewCore(opts.HotstuffID(), "hs", privKey, runtimeOpts...)
	// check if measurements should be enabled
	if w.measurementInterval > 0 {
		// Initializes the metrics modules internally.
		err = metrics.Enable(
			depsCore.EventLoop(),
			depsCore.Logger(),
			w.metricsLogger,
			depsCore.RuntimeCfg().ID(),
			w.measurementInterval,
			w.metrics...,
		)
		if err != nil {
			return nil, err
		}
	}
	creds := insecure.NewCredentials()
	// setup replica
	replicaOpts := []replica.Option{}
	if opts.GetUseTLS() {
		certificate, err := tls.X509KeyPair(opts.GetCertificate(), opts.GetCertificateKey())
		if err != nil {
			return nil, err
		}
		rootCAs := x509.NewCertPool()
		rootCAs.AppendCertsFromPEM(opts.GetCertificateAuthority())
		creds = credentials.NewTLS(&tls.Config{ // overwrite creds to a secure TLS version
			RootCAs:      rootCAs,
			Certificates: []tls.Certificate{certificate},
		})
		replicaOpts = append(replicaOpts, replica.WithTLS(certificate, rootCAs, creds))
	}
	replicaOpts = append(replicaOpts,
		replica.WithServerOptions(server.WithLatencies(opts.HotstuffID(), opts.GetLocations())),
	)
	sender := network.NewGorumsSender(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		creds,
	)
	depsSecure, err := wiring.NewSecurity(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		sender,
		opts.GetCrypto(),
		cert.WithCache(100), // TODO: consider making this configurable
	)
	if err != nil {
		return nil, err
	}
	consensusRules, viewStates, leaderRotation, disAgg, viewDuration, err := initConsensusModules(depsCore, depsSecure, sender, opts)
	if err != nil {
		return nil, err
	}
	return replica.New(
		depsCore,
		depsSecure,
		sender,
		viewStates,
		disAgg,
		leaderRotation,
		consensusRules,
		viewDuration,
		opts.GetBatchSize(),
		replicaOpts...,
	)
}

func initConsensusModules(depsCore *wiring.Core, depsSecure *wiring.Security, sender *network.GorumsSender, opts *orchestrationpb.ReplicaOpts) (modules.HotstuffRuleset, *protocol.ViewStates, modules.LeaderRotation, modules.DisseminatorAggregator, modules.ViewDuration, error) {
	consensusRules, err := wiring.NewConsensusRules(
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		opts.GetConsensus(),
	)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if byzStrategy := opts.GetByzantineStrategy(); byzStrategy != "" {
		byz, err := wiring.WrapByzantineStrategy(
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			depsSecure.BlockChain(),
			consensusRules,
			byzStrategy,
		)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		consensusRules = byz
	}
	viewStates, err := protocol.NewViewStates(
		depsSecure.BlockChain(),
		depsSecure.Authority(),
	)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	leaderRotation, err := wiring.NewLeaderRotation(
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecure.BlockChain(),
		viewStates,
		opts.GetLeaderRotation(),
		consensusRules.ChainLength(),
	)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	var disAgg modules.DisseminatorAggregator
	var viewDuration modules.ViewDuration
	if depsCore.RuntimeCfg().HasKauriTree() {
		viewDuration = viewduration.NewFixed(opts.GetInitialTimeout().AsDuration())
		disAgg = kauri.New(
			depsCore.Logger(),
			depsCore.EventLoop(),
			depsCore.RuntimeCfg(),
			depsSecure.BlockChain(),
			depsSecure.Authority(),
			kauri.NewExtendedGorumsSender(
				depsCore.EventLoop(),
				depsCore.RuntimeCfg(),
				sender,
			),
		)
	} else {
		viewDuration = viewduration.NewDynamic(viewduration.NewParams(
			opts.GetTimeoutSamples(),
			opts.GetInitialTimeout().AsDuration(),
			opts.GetMaxTimeout().AsDuration(),
			opts.GetTimeoutMultiplier(),
		))
		votingMachine := votingmachine.New(
			depsCore.Logger(),
			depsCore.EventLoop(),
			depsCore.RuntimeCfg(),
			depsSecure.BlockChain(),
			depsSecure.Authority(),
			viewStates,
		)
		disAgg = clique.New(
			depsCore.RuntimeCfg(),
			votingMachine,
			leaderRotation,
			sender,
		)
	}
	return consensusRules, viewStates, leaderRotation, disAgg, viewDuration, nil
}

// newTree creates a new tree based on the provided options. It uses the aggregation
// time option to determine the delay mode and initializes the tree with the specified
// branch factor, locations, position IDs, and delta duration.
func newTree(opts *orchestrationpb.ReplicaOpts) *tree.Tree {
	delayMode := tree.DelayTypeNone
	if opts.GetAggregationTime() {
		delayMode = tree.DelayTypeAggregation
	}
	return tree.NewDelayed(
		opts.HotstuffID(),
		delayMode,
		int(opts.GetBranchFactor()),
		latency.MatrixFrom(opts.GetLocations()),
		opts.TreePositionIDs(),
		opts.GetTreeDelta().AsDuration(),
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
		runtimeCfg := core.NewRuntimeConfig(hotstuff.ID(opts.GetID()), nil)
		logger := logging.New("cli" + strconv.Itoa(int(opts.GetID())))
		eventLoop := eventloop.New(logger, 1000)

		if w.measurementInterval > 0 {
			err := metrics.Enable(
				eventLoop,
				logger,
				w.metricsLogger,
				runtimeCfg.ID(),
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
			runtimeCfg,
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
