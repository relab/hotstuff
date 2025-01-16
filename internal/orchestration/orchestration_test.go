package orchestration_test

import (
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/relab/hotstuff/internal/config"
	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/internal/test"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics"
	"github.com/relab/iago/iagotest"
	"google.golang.org/protobuf/types/known/durationpb"
)

func makeClientOpts() *orchestrationpb.ClientOpts {
	return &orchestrationpb.ClientOpts{
		ConnectTimeout: durationpb.New(time.Second),
		MaxConcurrent:  250,
		PayloadSize:    100,
		RateLimit:      math.Inf(1),
		Timeout:        durationpb.New(500 * time.Millisecond),
	}
}

func makeCfg(
	replicas, clients int,
	consensusImpl,
	crypto string,
	leaderRotation string,
	byzantine string,
	branchFactor uint32,
	randomTree bool,
	mods ...string) *config.HostConfig {
	cfg := &config.HostConfig{
		Replicas:          replicas,
		Clients:           clients,
		ReplicaHosts:      []string{"localhost"},
		ClientHosts:       []string{"localhost"},
		TreePositions:     tree.DefaultTreePosUint32(replicas),
		RandomTree:        randomTree,
		BranchFactor:      branchFactor,
		Consensus:         consensusImpl,
		Crypto:            crypto,
		LeaderRotation:    leaderRotation,
		ByzantineStrategy: map[string][]uint32{byzantine: {1}},

		// Common default values:
		Duration:          5 * time.Second,
		BatchSize:         100,
		ConnectTimeout:    time.Second,
		ViewTimeout:       100 * time.Millisecond,
		DurationSamples:   1000,
		TimeoutMultiplier: 1.2,
		Modules:           mods,
		MaxConcurrent:     250,
		PayloadSize:       100,
		RateLimit:         math.Inf(1),
		ClientTimeout:     500 * time.Millisecond,
		TreeDelta:         30 * time.Millisecond,
	}
	if randomTree {
		rnd := rand.New(rand.NewSource(int64(rand.Uint64())))
		rnd.Shuffle(len(cfg.TreePositions), reflect.Swapper(cfg.TreePositions))
	}
	return cfg
}

func run(t *testing.T, cfg *config.HostConfig) {
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

	err = experiment.Run()
	if err != nil {
		t.Fatal(err)
	}

	err = <-c
	if err != nil {
		t.Fatal(err)
	}
}

func TestOrchestration(t *testing.T) {
	tests := []struct {
		consensus    string
		crypto       string
		byzantine    string
		mods         []string
		replicas     int
		branchFactor uint32
		randomTree   bool
	}{
		{consensus: "chainedhotstuff", crypto: "ecdsa", replicas: 4},
		{consensus: "chainedhotstuff", crypto: "eddsa", replicas: 4},
		{consensus: "chainedhotstuff", crypto: "bls12", replicas: 4},
		{consensus: "fasthotstuff", crypto: "ecdsa", replicas: 4},
		{consensus: "fasthotstuff", crypto: "eddsa", replicas: 4},
		{consensus: "fasthotstuff", crypto: "bls12", replicas: 4},
		{consensus: "simplehotstuff", crypto: "ecdsa", replicas: 4},
		{consensus: "simplehotstuff", crypto: "eddsa", replicas: 4},
		{consensus: "simplehotstuff", crypto: "bls12", replicas: 4},
		{consensus: "chainedhotstuff", crypto: "ecdsa", byzantine: "fork:1"},
		{consensus: "chainedhotstuff", crypto: "ecdsa", byzantine: "silence:1"},
		{consensus: "chainedhotstuff", crypto: "ecdsa", mods: []string{"kauri"}, replicas: 7, branchFactor: 2},
		{consensus: "chainedhotstuff", crypto: "bls12", mods: []string{"kauri"}, replicas: 7, branchFactor: 2},
		{consensus: "chainedhotstuff", crypto: "ecdsa", mods: []string{"kauri"}, replicas: 7, branchFactor: 2, randomTree: true},
		{consensus: "chainedhotstuff", crypto: "bls12", mods: []string{"kauri"}, replicas: 7, branchFactor: 2, randomTree: true},
	}

	for _, tt := range tests {
		t.Run(test.Name([]string{"consensus", "crypto", "byzantine", "mods"}, tt.consensus, tt.crypto, tt.byzantine, tt.mods), func(t *testing.T) {
			var leaderRotation string
			if slices.Contains(tt.mods, "kauri") {
				leaderRotation = "tree-leader"
			} else {
				leaderRotation = "round-robin"
			}
			cfg := makeCfg(
				tt.replicas, 2,
				tt.consensus,
				tt.crypto,
				leaderRotation,
				tt.byzantine,
				tt.branchFactor,
				tt.randomTree,
				tt.mods...,
			)
			run(t, cfg)
		})
	}
}

func TestDeployment(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" && runtime.GOOS != "linux" {
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
		fmt.Printf("Added worker host: %s\n", host)
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
	cfg := &config.HostConfig{
		Replicas:          numReplicas,
		Clients:           numClients,
		ReplicaHosts:      replicaHosts,
		ClientHosts:       clientHosts,
		Duration:          10 * time.Second,
		ConnectTimeout:    time.Second,
		MaxConcurrent:     250,
		PayloadSize:       100,
		RateLimit:         math.Inf(1),
		BatchSize:         100,
		ViewTimeout:       100 * time.Millisecond,
		DurationSamples:   1000,
		TimeoutMultiplier: 1.2,
		Consensus:         "chainedhotstuff",
		Crypto:            "ecdsa",
		LeaderRotation:    "round-robin",
	}

	experiment, err := orchestration.NewExperiment(
		cfg, // TODO: Find a cleaner approach to creating a config for a test case like this.
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
