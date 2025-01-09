package orchestration

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/config"
	"github.com/relab/hotstuff/internal/latency"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/logging"
	"google.golang.org/protobuf/proto"
)

// Experiment holds variables for an experiment.
type Experiment struct {
	replicaOpts *orchestrationpb.ReplicaOpts
	clientOpts  *orchestrationpb.ClientOpts

	logger logging.Logger

	duration time.Duration

	workers map[string]RemoteWorker
	output  string // path to output folder

	caKey *ecdsa.PrivateKey
	ca    *x509.Certificate

	hostCfg *config.HostConfig
}

// TODO: Attempt to reduce number of params
func NewExperiment(
	duration time.Duration,
	outputDir string,
	replicaOpts *orchestrationpb.ReplicaOpts,
	clientOpts *orchestrationpb.ClientOpts,
	cfg *config.HostConfig,
	workers map[string]RemoteWorker,
	logger logging.Logger,
) (*Experiment, error) {
	for _, location := range cfg.Locations {
		location, err := latency.ValidLocation(location)
		log.Printf("Experiment: Location found: %v", location)
		if err != nil {
			return nil, fmt.Errorf("invalid configuration: %v", err)
		}
	}
	return &Experiment{
		replicaOpts: replicaOpts,
		clientOpts:  clientOpts,
		logger:      logger,
		duration:    duration,
		output:      outputDir,
		workers:     workers,
		hostCfg:     cfg,
	}, nil
}

// Run runs the experiment.
func (e *Experiment) Run() (err error) {
	defer func() {
		qerr := e.quit()
		if err == nil {
			err = qerr
		}
	}()

	replicaMap := e.hostCfg.AssignReplicas(e.replicaOpts)
	clientIds := e.hostCfg.AssignClients()

	if e.output != "" {
		err = e.writeAssignmentsFile(replicaMap, clientIds)
		if err != nil {
			return err
		}
	}

	e.logger.Info("Creating replicas...")
	cfg, err := e.createReplicas(replicaMap)
	if err != nil {
		return fmt.Errorf("failed to create replicas: %w", err)
	}

	e.logger.Info("Starting replicas...")
	err = e.startReplicas(cfg, replicaMap)
	if err != nil {
		return fmt.Errorf("failed to start replicas: %w", err)
	}

	e.logger.Info("Starting clients...")
	err = e.startClients(cfg, clientIds)
	if err != nil {
		return fmt.Errorf("failed to start clients: %w", err)
	}

	time.Sleep(e.duration)

	e.logger.Info("Stopping clients...")
	err = e.stopClients(clientIds)
	if err != nil {
		return fmt.Errorf("failed to stop clients: %w", err)
	}

	wait := 5 * e.replicaOpts.GetInitialTimeout().AsDuration()
	e.logger.Infof("Waiting %s for replicas to finish.", wait)
	// give the replicas some time to commit the last batch
	time.Sleep(wait)

	e.logger.Info("Stopping replicas...")
	err = e.stopReplicas(replicaMap)
	if err != nil {
		return fmt.Errorf("failed to stop replicas: %w", err)
	}

	return nil
}

func (e *Experiment) putReplicaOpt(toMap map[uint32]*orchestrationpb.ReplicaOpts, host string, opt *orchestrationpb.ReplicaOpts) error {
	opt.CertificateAuthority = keygen.CertToPEM(e.ca)
	validFor := []string{"localhost", "127.0.0.1", "127.0.1.1", host}
	ips, err := net.LookupIP(host)
	if err == nil {
		for _, ip := range ips {
			// TODO: Not using internal addr anymore, but check if this is needed.
			if ipStr := ip.String(); ipStr != host /*&& ipStr != internalAddr*/ {
				validFor = append(validFor, ipStr)
			}
		}
	}

	keyChain, err := keygen.GenerateKeyChain(hotstuff.ID(opt.ID), validFor, e.replicaOpts.Crypto, e.ca, e.caKey)
	if err != nil {
		return fmt.Errorf("failed to generate keychain: %w", err)
	}
	opt.PrivateKey = keyChain.PrivateKey
	opt.PublicKey = keyChain.PublicKey
	opt.Certificate = keyChain.Certificate
	opt.CertificateKey = keyChain.CertificateKey

	toMap[opt.ID] = opt
	return nil
}

func (e *Experiment) createReplicas(replicaMap config.ReplicaMap) (cfg *orchestrationpb.ReplicaConfiguration, err error) {
	e.caKey, e.ca, err = keygen.GenerateCA()
	if err != nil {
		return nil, err
	}

	cfg = &orchestrationpb.ReplicaConfiguration{Replicas: make(map[uint32]*orchestrationpb.ReplicaInfo)}

	for host, opts := range replicaMap {
		worker := e.workers[host]
		req := &orchestrationpb.CreateReplicaRequest{Replicas: make(map[uint32]*orchestrationpb.ReplicaOpts)}

		for _, opt := range opts {
			err = e.putReplicaOpt(req.Replicas, host, opt)
			if err != nil {
				return nil, err
			}
		}

		wcfg, err := worker.CreateReplica(req)
		if err != nil {
			return nil, err
		}

		for id, replicaCfg := range wcfg.GetReplicas() {
			replicaCfg.Address = host
			e.logger.Debugf("Replica %d: Address: %s, PublicKey: %t, ReplicaPort: %d, ClientPort: %d",
				id, replicaCfg.Address, len(replicaCfg.PublicKey) > 0, replicaCfg.ReplicaPort, replicaCfg.ClientPort)
			cfg.Replicas[id] = replicaCfg
		}
	}

	return cfg, nil
}

type assignmentsFileContents struct {
	// the host associated with each replica.
	HostsToReplicas map[string][]hotstuff.ID
	// the host associated with each client.
	HostsToClients map[string][]hotstuff.ID
}

func (e *Experiment) writeAssignmentsFile(replicaMap config.ReplicaMap, clientIDs config.ClientMap) (err error) {
	f, err := os.OpenFile(filepath.Join(e.output, "hosts.json"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "\t")

	replicaIDs := make(map[string][]hotstuff.ID)
	for host, opts := range replicaMap {
		replicaIDs[host] = make([]hotstuff.ID, 0, len(opts))
		for _, opt := range opts {
			replicaIDs[host] = append(replicaIDs[host], opt.HotstuffID())
		}
	}

	return enc.Encode(assignmentsFileContents{
		HostsToReplicas: replicaIDs,
		HostsToClients:  clientIDs,
	})
}

func (e *Experiment) startReplicas(cfg *orchestrationpb.ReplicaConfiguration, replicaMap config.ReplicaMap) (err error) {
	errs := make(chan error)
	for host, worker := range e.workers {
		go func(host string, worker RemoteWorker) {
			req := &orchestrationpb.StartReplicaRequest{
				Configuration: cfg.GetReplicas(),
				IDs:           replicaIDsToU32(host, replicaMap),
			}
			_, err := worker.StartReplica(req)
			errs <- err
		}(host, worker)
	}
	for range e.workers {
		err = errors.Join(err, <-errs)
	}
	return err
}

func (e *Experiment) stopReplicas(replicaMap config.ReplicaMap) error {
	responses := make([]*orchestrationpb.StopReplicaResponse, 0)
	for host, worker := range e.workers {
		req := &orchestrationpb.StopReplicaRequest{IDs: replicaIDsToU32(host, replicaMap)}
		res, err := worker.StopReplica(req)
		if err != nil {
			return err
		}
		responses = append(responses, res)
	}
	return verifyStopResponses(responses)
}

func verifyStopResponses(responses []*orchestrationpb.StopReplicaResponse) error {
	results := make(map[uint32][][]byte)
	for _, response := range responses {
		commandCount := response.GetCounts()
		hashes := response.GetHashes()
		for id, count := range commandCount {
			if len(results[count]) == 0 {
				results[count] = make([][]byte, 0)
			}
			results[count] = append(results[count], hashes[id])
		}
	}
	for cmdCount, hashes := range results {
		firstHash := hashes[0]
		for _, hash := range hashes {
			if !bytes.Equal(firstHash, hash) {
				return fmt.Errorf("hash mismatch at command: %d", cmdCount)
			}
		}
	}
	return nil
}

func (e *Experiment) startClients(cfg *orchestrationpb.ReplicaConfiguration, clientMap config.ClientMap) error {
	for host, worker := range e.workers {
		req := &orchestrationpb.StartClientRequest{}
		req.Clients = make(map[uint32]*orchestrationpb.ClientOpts)
		req.Configuration = cfg.GetReplicas()
		req.CertificateAuthority = keygen.CertToPEM(e.ca)
		for _, id := range clientMap[host] {
			clientOpts := proto.Clone(e.clientOpts).(*orchestrationpb.ClientOpts)
			clientOpts.ID = uint32(id)
			req.Clients[uint32(id)] = clientOpts
		}
		_, err := worker.StartClient(req)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Experiment) stopClients(clientMap config.ClientMap) error {
	for host, worker := range e.workers {
		req := &orchestrationpb.StopClientRequest{}
		req.IDs = clientIDsToU32(host, clientMap)
		_, err := worker.StopClient(req)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Experiment) quit() error {
	for _, worker := range e.workers {
		err := worker.Quit()
		if err != nil {
			return err
		}
	}
	return nil
}

func replicaIDsToU32(host string, replicaMap config.ReplicaMap) []uint32 {
	var ids []uint32
	for _, opts := range replicaMap[host] {
		ids = append(ids, uint32(opts.ID))
	}
	return ids
}

func clientIDsToU32(host string, clientIDs config.ClientMap) []uint32 {
	newList := make([]uint32, 0, len(clientIDs))
	for _, id := range clientIDs[host] {
		newList = append(newList, uint32(id))
	}
	return newList
}
