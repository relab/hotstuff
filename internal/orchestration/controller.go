package orchestration

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"go.uber.org/multierr"
	"google.golang.org/protobuf/types/known/durationpb"
)

// HostConfig specifies the number of replicas and clients that should be started on a specific host.
type HostConfig struct {
	Replicas int
	Clients  int
}

// Experiment holds variables for an experiment.
type Experiment struct {
	NumReplicas       int
	NumClients        int
	BatchSize         int
	PayloadSize       int
	MaxConcurrent     int
	Duration          time.Duration
	ConnectTimeout    time.Duration
	ViewTimeout       time.Duration
	TimoutSamples     int
	TimeoutMultiplier float32
	Consensus         string
	Crypto            string
	LeaderRotation    string
	RateLimit         float64
	RateStep          float64
	RateStepInterval  time.Duration

	Hosts       map[string]RemoteWorker
	HostConfigs map[string]HostConfig

	// the host associated with each replica.
	hostsToReplicas map[string][]hotstuff.ID
	// the host associated with each client.
	hostsToClients map[string][]hotstuff.ID
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

	cfg, err := e.createReplicas()
	if err != nil {
		return fmt.Errorf("failed to create replicas: %w", err)
	}

	err = e.startReplicas(cfg)
	if err != nil {
		return fmt.Errorf("failed to start replicas: %w", err)
	}

	err = e.startClients(cfg)
	if err != nil {
		return fmt.Errorf("failed to start clients: %w", err)
	}

	time.Sleep(e.Duration)

	err = e.stopClients()
	if err != nil {
		return fmt.Errorf("failed to stop clients: %w", err)
	}

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
		req := &orchestrationpb.CreateReplicaRequest{Replicas: make(map[uint32]*orchestrationpb.ReplicaOpts)}
		for _, id := range e.hostsToReplicas[host] {
			opts := orchestrationpb.ReplicaOpts{
				CertificateAuthority: keygen.CertToPEM(e.ca),
				UseTLS:               true,
				Crypto:               e.Crypto,
				Consensus:            e.Consensus,
				LeaderRotation:       e.LeaderRotation,
				BatchSize:            uint32(e.BatchSize),
				BlockCacheSize:       uint32(5 * e.NumReplicas),
				InitialTimeout:       float32(e.ViewTimeout) / float32(time.Millisecond),
				TimeoutSamples:       uint32(e.TimoutSamples),
				TimeoutMultiplier:    e.TimeoutMultiplier,
				ConnectTimeout:       float32(e.ConnectTimeout / time.Millisecond),
			}

			// the generated certificate should be valid for the hostname and its ip addresses.
			validFor := []string{host}
			ips, err := net.LookupIP(host)
			if err == nil {
				for _, ip := range ips {
					validFor = append(validFor, ip.String())
				}
			}

			keyChain, err := keygen.GenerateKeyChain(id, validFor, e.Crypto, e.ca, e.caKey)
			if err != nil {
				return nil, fmt.Errorf("failed to generate keychain: %w", err)
			}

			opts.ID = uint32(id)
			opts.PrivateKey = keyChain.PrivateKey
			opts.PublicKey = keyChain.PublicKey
			opts.Certificate = keyChain.Certificate
			opts.CertificateKey = keyChain.CertificateKey
			req.Replicas[uint32(id)] = &opts
		}
		wcfg, err := worker.CreateReplica(req)
		if err != nil {
			return nil, err
		}

		for id, replicaCfg := range wcfg.GetReplicas() {
			replicaCfg.Address = host
			cfg.Replicas[id] = replicaCfg
		}
	}

	return cfg, nil
}

// assignReplicasAndClients assigns replica and client ids to each host,
// based on the requested amount of replicas/clients and the assignments for each host.
func (e *Experiment) assignReplicasAndClients() (err error) {
	e.hostsToReplicas = make(map[string][]hotstuff.ID)
	e.hostsToClients = make(map[string][]hotstuff.ID)

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
		)
		if hostCfg, ok := e.HostConfigs[host]; ok {
			numReplicas = hostCfg.Replicas
			numClients = hostCfg.Clients
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
		}

		for i := 0; i < numReplicas; i++ {
			e.hostsToReplicas[host] = append(e.hostsToReplicas[host], nextReplicaID)
			log.Printf("replica %d assigned to host %s", nextReplicaID, host)
			nextReplicaID++
		}

		for i := 0; i < numClients; i++ {
			e.hostsToClients[host] = append(e.hostsToClients[host], nextClientID)
			log.Printf("client %d assigned to host %s", nextClientID, host)
			nextClientID++
		}
	}
	// TODO: warn if not all clients/replicas were assigned
	return nil
}

func (e *Experiment) startReplicas(cfg *orchestrationpb.ReplicaConfiguration) (err error) {
	errors := make(chan error)
	for host, worker := range e.Hosts {
		go func(host string, worker RemoteWorker) {
			req := &orchestrationpb.StartReplicaRequest{
				Configuration: cfg.GetReplicas(),
				IDs:           getIDs(host, e.hostsToReplicas),
			}
			_, err := worker.StartReplica(req)
			errors <- err
		}(host, worker)
	}
	for range e.Hosts {
		err = multierr.Append(err, <-errors)
	}
	return nil
}

func (e *Experiment) stopReplicas() error {
	hashes := make(map[uint32][]byte)
	for host, worker := range e.Hosts {
		req := &orchestrationpb.StopReplicaRequest{IDs: getIDs(host, e.hostsToReplicas)}
		res, err := worker.StopReplica(req)
		if err != nil {
			return err
		}
		for id, hash := range res.GetHashes() {
			hashes[id] = hash
		}
	}
	var cmp []byte
	for _, hash := range hashes {
		if cmp == nil {
			cmp = hash
		}
		if !bytes.Equal(cmp, hash) {
			return fmt.Errorf("hash mismatch")
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
			req.Clients[uint32(id)] = &orchestrationpb.ClientOpts{
				ID:               uint32(id),
				UseTLS:           true,
				MaxConcurrent:    uint32(e.MaxConcurrent),
				PayloadSize:      uint32(e.PayloadSize),
				ConnectTimeout:   float32(e.ConnectTimeout / time.Millisecond),
				RateLimit:        e.RateLimit,
				RateStep:         e.RateStep,
				RateStepInterval: durationpb.New(e.RateStepInterval),
			}
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
