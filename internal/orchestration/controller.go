package orchestration

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/logging"
	"google.golang.org/protobuf/proto"
)

// HostConfig specifies the number of replicas and clients that should be started on a specific host.
type HostConfig struct {
	Name            string
	Clients         int
	Replicas        int
	Location        string
	InternalAddress string `mapstructure:"internal-address"`
}

// Experiment holds variables for an experiment.
type Experiment struct {
	*orchestrationpb.ReplicaOpts
	*orchestrationpb.ClientOpts

	Logger logging.Logger

	NumReplicas int
	NumClients  int
	Duration    time.Duration

	Hosts       map[string]RemoteWorker
	HostConfigs map[string]HostConfig
	Byzantine   map[string]int // number of replicas to assign to each byzantine strategy
	Output      string         // path to output folder

	// the host associated with each replica.
	hostsToReplicas map[string][]hotstuff.ID
	// the host associated with each client.
	hostsToClients map[string][]hotstuff.ID
	replicaOpts    map[hotstuff.ID]*orchestrationpb.ReplicaOpts
	caKey          *ecdsa.PrivateKey
	ca             *x509.Certificate
}

// Run runs the experiment.
func (e *Experiment) Run() (err error) {
	defer func() {
		qerr := e.quit()
		if err == nil {
			err = qerr
		}
	}()

	err = e.assignReplicasAndClients()
	if err != nil {
		return err
	}

	if e.Output != "" {
		err = e.writeAssignmentsFile()
		if err != nil {
			return err
		}
	}

	e.Logger.Info("Creating replicas...")
	cfg, err := e.createReplicas()
	if err != nil {
		return fmt.Errorf("failed to create replicas: %w", err)
	}

	e.Logger.Info("Starting replicas...")
	err = e.startReplicas(cfg)
	if err != nil {
		return fmt.Errorf("failed to start replicas: %w", err)
	}

	e.Logger.Info("Starting clients...")
	err = e.startClients(cfg)
	if err != nil {
		return fmt.Errorf("failed to start clients: %w", err)
	}

	time.Sleep(e.Duration)

	e.Logger.Info("Stopping clients...")
	err = e.stopClients()
	if err != nil {
		return fmt.Errorf("failed to stop clients: %w", err)
	}

	wait := 5 * e.ReplicaOpts.GetInitialTimeout().AsDuration()
	e.Logger.Infof("Waiting %s for replicas to finish.", wait)
	// give the replicas some time to commit the last batch
	time.Sleep(wait)

	e.Logger.Info("Stopping replicas...")
	err = e.stopReplicas()
	if err != nil {
		return fmt.Errorf("failed to stop replicas: %w", err)
	}

	return nil
}

func (e *Experiment) createReplicas() (cfg *orchestrationpb.ReplicaConfiguration, err error) {
	e.caKey, e.ca, err = keygen.GenerateCA()
	if err != nil {
		return nil, err
	}

	cfg = &orchestrationpb.ReplicaConfiguration{Replicas: make(map[uint32]*orchestrationpb.ReplicaInfo)}

	for host, worker := range e.Hosts {
		internalAddr := e.HostConfigs[host].InternalAddress

		req := &orchestrationpb.CreateReplicaRequest{Replicas: make(map[uint32]*orchestrationpb.ReplicaOpts)}
		for _, id := range e.hostsToReplicas[host] {
			opts := e.replicaOpts[id]
			opts.CertificateAuthority = keygen.CertToPEM(e.ca)

			// the generated certificate should be valid for the hostname and its ip addresses.
			validFor := []string{"localhost", "127.0.0.1", "127.0.1.1", host}
			ips, err := net.LookupIP(host)
			if err == nil {
				for _, ip := range ips {
					if ipStr := ip.String(); ipStr != host && ipStr != internalAddr {
						validFor = append(validFor, ipStr)
					}
				}
			}

			// add the internal address as well
			if internalAddr != "" {
				validFor = append(validFor, internalAddr)
			}

			keyChain, err := keygen.GenerateKeyChain(id, validFor, e.Crypto, e.ca, e.caKey)
			if err != nil {
				return nil, fmt.Errorf("failed to generate keychain: %w", err)
			}
			opts.PrivateKey = keyChain.PrivateKey
			opts.PublicKey = keyChain.PublicKey
			opts.Certificate = keyChain.Certificate
			opts.CertificateKey = keyChain.CertificateKey
			req.Replicas[opts.ID] = opts
		}
		wcfg, err := worker.CreateReplica(req)
		if err != nil {
			return nil, err
		}

		for id, replicaCfg := range wcfg.GetReplicas() {
			if internalAddr != "" {
				replicaCfg.Address = internalAddr
			} else {
				replicaCfg.Address = host
			}
			e.Logger.Debugf("Address for replica %d: %s", id, replicaCfg.Address)
			cfg.Replicas[id] = replicaCfg
		}
	}

	return cfg, nil
}

// assignReplicasAndClients assigns replica and client ids to each host,
// based on the requested amount of replicas/clients and the assignments for each host.
func (e *Experiment) assignReplicasAndClients() (err error) {
	e.hostsToReplicas = make(map[string][]hotstuff.ID)
	e.replicaOpts = make(map[hotstuff.ID]*orchestrationpb.ReplicaOpts)
	e.hostsToClients = make(map[string][]hotstuff.ID)
	replicaLocationInfo := make(map[uint32]string)
	nextReplicaID := hotstuff.ID(1)
	nextClientID := hotstuff.ID(1)

	// number of replicas that should be auto assigned
	remainingReplicas := e.NumReplicas
	remainingClients := e.NumClients

	// how many workers that should be auto assigned
	autoConfig := len(e.Hosts)

	// determine how many replicas should be assigned automatically
	for _, hostCfg := range e.HostConfigs {
		// TODO: ensure that this host is part of e.Hosts
		if hostCfg.Clients|hostCfg.Replicas == 0 {
			// if both are zero, we'll autoconfigure this host.
			continue
		}
		remainingReplicas -= hostCfg.Replicas
		remainingClients -= hostCfg.Clients
		autoConfig--
	}

	var (
		replicasPerNode   int
		remainderReplicas int
		clientsPerNode    int
		remainderClients  int
	)

	if autoConfig > 0 {
		replicasPerNode = remainingReplicas / autoConfig
		remainderReplicas = remainingReplicas % autoConfig
		clientsPerNode = remainingClients / autoConfig
		remainderClients = remainingClients % autoConfig
	}

	// ensure that we have not assigned more replicas or clients than requested
	if remainingReplicas < 0 {
		return fmt.Errorf(
			"invalid replica configuration: %d replicas requested, but host configuration specifies %d",
			e.NumReplicas, e.NumReplicas-remainingReplicas,
		)
	}
	if remainingClients < 0 {
		return fmt.Errorf(
			"invalid client configuration: %d clients requested, but host configuration specifies %d",
			e.NumClients, e.NumClients-remainingClients,
		)
	}

	for host := range e.Hosts {
		var (
			numReplicas int
			numClients  int
			location    string
		)
		if hostCfg, ok := e.HostConfigs[host]; ok && hostCfg.Clients|hostCfg.Replicas != 0 {
			numReplicas = hostCfg.Replicas
			numClients = hostCfg.Clients
			location = hostCfg.Location
		} else {
			numReplicas = replicasPerNode
			remainingReplicas -= replicasPerNode
			if remainderReplicas > 0 {
				numReplicas++
				remainderReplicas--
				remainingReplicas--
			}
			numClients = clientsPerNode
			remainingClients -= clientsPerNode
			if remainderClients > 0 {
				numClients++
				remainderClients--
				remainingClients--
			}
			location = "default"
		}

		for i := 0; i < numReplicas; i++ {
			var byzantineStrategy string
			for strategy, count := range e.Byzantine {
				if count > 0 {
					e.Byzantine[strategy]--
					byzantineStrategy = strategy
				}
			}

			// copy the replica opts
			replicaOpts := proto.Clone(e.ReplicaOpts).(*orchestrationpb.ReplicaOpts)
			replicaOpts.ID = uint32(nextReplicaID)
			replicaOpts.ByzantineStrategy = byzantineStrategy
			replicaLocationInfo[replicaOpts.ID] = location
			// all replicaOpts share the same LocationInfo map, which is progressively updated
			replicaOpts.LocationInfo = replicaLocationInfo
			e.hostsToReplicas[host] = append(e.hostsToReplicas[host], nextReplicaID)
			e.replicaOpts[nextReplicaID] = replicaOpts
			e.Logger.Infof("replica %d assigned to host %s", nextReplicaID, host)
			nextReplicaID++
		}

		for i := 0; i < numClients; i++ {
			e.hostsToClients[host] = append(e.hostsToClients[host], nextClientID)
			e.Logger.Infof("client %d assigned to host %s", nextClientID, host)
			nextClientID++
		}
	}

	// TODO: warn if not all clients/replicas were assigned
	return nil
}

type assignmentsFileContents struct {
	// the host associated with each replica.
	HostsToReplicas map[string][]hotstuff.ID
	// the host associated with each client.
	HostsToClients map[string][]hotstuff.ID
}

func (e *Experiment) writeAssignmentsFile() (err error) {
	f, err := os.OpenFile(filepath.Join(e.Output, "hosts.json"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
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
	return enc.Encode(assignmentsFileContents{
		HostsToReplicas: e.hostsToReplicas,
		HostsToClients:  e.hostsToClients,
	})
}

func (e *Experiment) startReplicas(cfg *orchestrationpb.ReplicaConfiguration) (err error) {
	errs := make(chan error)
	for host, worker := range e.Hosts {
		go func(host string, worker RemoteWorker) {
			req := &orchestrationpb.StartReplicaRequest{
				Configuration: cfg.GetReplicas(),
				IDs:           getIDs(host, e.hostsToReplicas),
			}
			_, err := worker.StartReplica(req)
			errs <- err
		}(host, worker)
	}
	for range e.Hosts {
		err = errors.Join(err, <-errs)
	}
	return err
}

func (e *Experiment) stopReplicas() error {
	responses := make([]*orchestrationpb.StopReplicaResponse, 0)
	for host, worker := range e.Hosts {
		req := &orchestrationpb.StopReplicaRequest{IDs: getIDs(host, e.hostsToReplicas)}
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

func (e *Experiment) startClients(cfg *orchestrationpb.ReplicaConfiguration) error {
	for host, worker := range e.Hosts {
		req := &orchestrationpb.StartClientRequest{}
		req.Clients = make(map[uint32]*orchestrationpb.ClientOpts)
		req.Configuration = cfg.GetReplicas()
		req.CertificateAuthority = keygen.CertToPEM(e.ca)
		for _, id := range e.hostsToClients[host] {
			clientOpts := proto.Clone(e.ClientOpts).(*orchestrationpb.ClientOpts)
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

func (e *Experiment) stopClients() error {
	for host, worker := range e.Hosts {
		req := &orchestrationpb.StopClientRequest{}
		req.IDs = getIDs(host, e.hostsToClients)
		_, err := worker.StopClient(req)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Experiment) quit() error {
	for _, worker := range e.Hosts {
		err := worker.Quit()
		if err != nil {
			return err
		}
	}
	return nil
}

func getIDs(host string, m map[string][]hotstuff.ID) []uint32 {
	var ids []uint32
	for _, id := range m[host] {
		ids = append(ids, uint32(id))
	}
	return ids
}
