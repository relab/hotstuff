package orchestration_test

import (
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/config"
	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/internal/test"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/metrics"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/security/crypto"
	"github.com/relab/iago/iagotest"
)

func makeCfg(
	replicas, clients int,
	consensusImpl, crypto, leaderRotation string,
	byzantine map[string][]uint32,
	branchFactor uint32,
	randomTree bool,
	kauri bool,
) *config.ExperimentConfig {
	useAggQC := consensusImpl == rules.ModuleNameFastHotstuff
	cfg := &config.ExperimentConfig{
		Replicas:          replicas,
		Clients:           clients,
		TreePositions:     tree.DefaultTreePosUint32(replicas),
		RandomTree:        randomTree,
		BranchFactor:      branchFactor,
		Consensus:         consensusImpl,
		Communication:     comm.ModuleNameClique,
		Crypto:            crypto,
		LeaderRotation:    leaderRotation,
		ByzantineStrategy: byzantine,
		Kauri:             kauri,
		UseAggQC:          useAggQC,

		// Common default values:
		ReplicaHosts:      []string{"localhost"},
		ClientHosts:       []string{"localhost"},
		Duration:          5 * time.Second,
		BatchSize:         100,
		ConnectTimeout:    time.Second,
		ViewTimeout:       100 * time.Millisecond,
		DurationSamples:   1000,
		TimeoutMultiplier: 1.2,
		MaxConcurrent:     250,
		PayloadSize:       100,
		RateLimit:         math.Inf(1),
		ClientTimeout:     500 * time.Millisecond,
		TreeDelta:         30 * time.Millisecond,
	}
	if randomTree {
		tree.Shuffle(cfg.TreePositions)
	}
	return cfg
}

func run(t *testing.T, cfg *config.ExperimentConfig) {
	t.Helper()

	controllerStream, workerStream := net.Pipe()
	workerProxy := orchestration.NewRemoteWorker(protostream.NewWriter(controllerStream), protostream.NewReader(controllerStream))
	worker := orchestration.NewWorker(protostream.NewWriter(workerStream), protostream.NewReader(workerStream), metrics.NopLogger(), nil, 0)

	experiment, err := orchestration.NewExperiment(
		cfg,
		map[string]orchestration.RemoteWorker{"localhost": workerProxy},
		logging.New("ctrl"),
	)
	if err != nil {
		t.Fatal(err)
	}

	c := make(chan error)
	go func() {
		c <- worker.Run()
	}()

	if err = experiment.Run(); err != nil {
		t.Fatal(err)
	}
	if err = <-c; err != nil {
		t.Fatal(err)
	}
}

func TestOrchestration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping slow test in short mode")
	}
	fork := map[string][]uint32{"fork": {1}}
	silence := map[string][]uint32{"silence": {1}}

	tests := []struct {
		consensus    string
		crypto       string
		byzantine    map[string][]uint32
		kauri        bool
		replicas     int
		branchFactor uint32
		randomTree   bool
	}{
		{consensus: rules.ModuleNameChainedHotstuff, crypto: crypto.ModuleNameECDSA, replicas: 4},
		{consensus: rules.ModuleNameChainedHotstuff, crypto: crypto.ModuleNameEDDSA, replicas: 4},
		{consensus: rules.ModuleNameChainedHotstuff, crypto: crypto.ModuleNameBLS12, replicas: 4},
		{consensus: rules.ModuleNameFastHotstuff, crypto: crypto.ModuleNameECDSA, replicas: 4},
		{consensus: rules.ModuleNameFastHotstuff, crypto: crypto.ModuleNameEDDSA, replicas: 4},
		{consensus: rules.ModuleNameFastHotstuff, crypto: crypto.ModuleNameBLS12, replicas: 4},
		{consensus: rules.ModuleNameSimpleHotStuff, crypto: crypto.ModuleNameECDSA, replicas: 4},
		{consensus: rules.ModuleNameSimpleHotStuff, crypto: crypto.ModuleNameEDDSA, replicas: 4},
		{consensus: rules.ModuleNameSimpleHotStuff, crypto: crypto.ModuleNameBLS12, replicas: 4},
		{consensus: rules.ModuleNameChainedHotstuff, crypto: crypto.ModuleNameECDSA, byzantine: fork, replicas: 4},
		{consensus: rules.ModuleNameChainedHotstuff, crypto: crypto.ModuleNameECDSA, byzantine: silence, replicas: 4},
		{consensus: rules.ModuleNameChainedHotstuff, crypto: crypto.ModuleNameECDSA, kauri: true, replicas: 7, branchFactor: 2},
		{consensus: rules.ModuleNameChainedHotstuff, crypto: crypto.ModuleNameBLS12, kauri: true, replicas: 7, branchFactor: 2},
		{consensus: rules.ModuleNameChainedHotstuff, crypto: crypto.ModuleNameECDSA, kauri: true, replicas: 7, branchFactor: 2, randomTree: true},
		{consensus: rules.ModuleNameChainedHotstuff, crypto: crypto.ModuleNameBLS12, kauri: true, replicas: 7, branchFactor: 2, randomTree: true},
	}

	for _, tt := range tests {
		t.Run(test.Name([]string{"consensus", "crypto", "byzantine", "kauri"}, tt.consensus, tt.crypto, tt.byzantine, tt.kauri), func(t *testing.T) {
			var leaderRotation string
			if tt.kauri {
				leaderRotation = leaderrotation.ModuleNameTree
			} else {
				leaderRotation = leaderrotation.ModuleNameRoundRobin
			}
			cfg := makeCfg(
				tt.replicas, 2,
				tt.consensus,
				tt.crypto,
				leaderRotation,
				tt.byzantine,
				tt.branchFactor,
				tt.randomTree,
				tt.kauri,
			)
			run(t, cfg)
		})
	}
}

func TestDeployment(t *testing.T) {
	t.Skip("skipping... TODO: remove later")
	if os.Getenv("GITHUB_ACTIONS") != "" && runtime.GOOS != "linux" {
		t.Log("WARNING: This test was skipped.")
		t.Skip("GitHub Actions only supports linux containers on linux runners.")
	}

	numReplicas := 4
	numClients := 2
	numHosts := numReplicas + numClients
	exe := compileBinary(t)
	g := iagotest.CreateSSHGroup(t, numHosts, true)

	sessions, err := orchestration.Deploy(g, orchestration.DeployConfig{
		ExePath:  exe,
		LogLevel: "info",
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(len(sessions))
	workers := make(map[string]orchestration.RemoteWorker)
	for host, session := range sessions {
		t.Logf("Added worker host: %s", host)
		workers[host] = orchestration.NewRemoteWorker(protostream.NewWriter(session.Stdin()), protostream.NewReader(session.Stdout()))
		go func(session orchestration.WorkerSession) {
			_, err := io.Copy(os.Stderr, session.Stderr())
			if err != nil {
				t.Error("failed to copy stderr:", err)
			}
			wg.Done()
		}(session)
	}

	// Put all hostnames into a string list.
	allHosts := make([]string, 0, numHosts)
	for host := range workers {
		allHosts = append(allHosts, host)
	}

	// Pop any hostname and add them to a separate list for replicas.
	replicaHosts := make([]string, 0, numReplicas)
	for range numReplicas {
		popped := allHosts[0]
		replicaHosts = append(replicaHosts, popped)
		allHosts = allHosts[1:]
	}

	// Pop any hostname and add them to a separate list for clients.
	clientHosts := make([]string, 0, numClients)
	for range numClients {
		popped := allHosts[0]
		clientHosts = append(clientHosts, popped)
		allHosts = allHosts[1:]
	}
	// Add all replica and client hostnames (that came from workers) separately
	// to the config.
	cfg := &config.ExperimentConfig{
		Replicas:          numReplicas,
		Clients:           numClients,
		ReplicaHosts:      replicaHosts,
		ClientHosts:       clientHosts,
		Duration:          10 * time.Second,
		ClientTimeout:     500 * time.Millisecond,
		ConnectTimeout:    time.Second,
		MaxConcurrent:     250,
		PayloadSize:       100,
		RateLimit:         math.Inf(1),
		BatchSize:         100,
		ViewTimeout:       100 * time.Millisecond,
		DurationSamples:   1000,
		TimeoutMultiplier: 1.2,
		Consensus:         rules.ModuleNameChainedHotstuff,
		Crypto:            crypto.ModuleNameECDSA,
		LeaderRotation:    "round-robin",
	}

	experiment, err := orchestration.NewExperiment(
		cfg,
		workers,
		logging.New("ctrl"),
	)
	if err != nil {
		t.Fatal(err)
	}

	if err = experiment.Run(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}

func findProjectRoot(t *testing.T) string {
	// The path to the parent folder of this file.
	// Will need to be updated if the package is ever moved.
	packagePath := filepath.Join("internal", "orchestration")

	_, curFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller info")
	}

	base := filepath.Dir(curFile)
	if !strings.HasSuffix(base, packagePath) {
		t.Fatalf("expected the current file to be within '%s'", packagePath)
	}

	root, err := filepath.Abs(strings.TrimSuffix(base, packagePath))
	if err != nil {
		t.Fatal("failed to get absolute path of project root: ", err)
	}

	return root
}

func compileBinary(t *testing.T) string {
	dir := t.TempDir()
	exe := filepath.Join(dir, "hotstuff")
	cmd := exec.Command("go", "build", "-o", exe, "./cmd/hotstuff")
	cmd.Dir = findProjectRoot(t)
	// assume docker host is using the same architecture
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("%s", out)
		t.Fatal(err)
	}
	return exe
}
