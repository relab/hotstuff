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

// makeReplicaOpts creates a new ReplicaOpts with the given parameters.
func makeReplicaOpts(consensusImpl, crypto, byzantine string, mods []string) *orchestrationpb.ReplicaOpts {
	return &orchestrationpb.ReplicaOpts{
		BatchSize:         100,
		ConnectTimeout:    durationpb.New(time.Second),
		InitialTimeout:    durationpb.New(100 * time.Millisecond),
		TimeoutSamples:    1000,
		TimeoutMultiplier: 1.2,
		Consensus:         consensusImpl,
		Crypto:            crypto,
		LeaderRotation:    "round-robin",
		Modules:           mods,
		ByzantineStrategy: byzantine,
	}
}

func makeTreeReplicaOpts(consensusImpl, crypto string, mods []string, replicas, bf int, random bool) *orchestrationpb.ReplicaOpts {
	treePos := tree.DefaultTreePosUint32(replicas)
	if random {
		rnd := rand.New(rand.NewSource(int64(rand.Uint64())))
		rnd.Shuffle(len(treePos), reflect.Swapper(treePos))
	}

	return &orchestrationpb.ReplicaOpts{
		BatchSize:         100,
		ConnectTimeout:    durationpb.New(time.Second),
		InitialTimeout:    durationpb.New(100 * time.Millisecond),
		TimeoutSamples:    1000,
		TimeoutMultiplier: 1.2,
		Consensus:         consensusImpl,
		Crypto:            crypto,
		LeaderRotation:    "tree-leader",
		Modules:           mods,
		ByzantineStrategy: "", // TODO currently kauri does not support byzantine replicas
		BranchFactor:      uint32(bf),
		TreePositions:     treePos,
		TreeDelta:         durationpb.New(30 * time.Millisecond),
	}
}

func makeClientOpts() *orchestrationpb.ClientOpts {
	return &orchestrationpb.ClientOpts{
		ConnectTimeout: durationpb.New(time.Second),
		MaxConcurrent:  250,
		PayloadSize:    100,
		RateLimit:      math.Inf(1),
		Timeout:        durationpb.New(500 * time.Millisecond),
	}
}

func run(t *testing.T, replicaOpts *orchestrationpb.ReplicaOpts) {
	t.Helper()

	controllerStream, workerStream := net.Pipe()
	workerProxy := orchestration.NewRemoteWorker(protostream.NewWriter(controllerStream), protostream.NewReader(controllerStream))
	worker := orchestration.NewWorker(protostream.NewWriter(workerStream), protostream.NewReader(workerStream), metrics.NopLogger(), nil, 0)

	cfg := config.NewLocal(7, 2)
	cfg.TreePositions = replicaOpts.TreePositions
	cfg.BranchFactor = replicaOpts.BranchFactor
	cfg.TreeDelta = replicaOpts.TreeDelta.AsDuration()

	experiment, err := orchestration.NewExperiment(
		5*time.Second,
		"",
		replicaOpts,
		makeClientOpts(),
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
		consensus  string
		crypto     string
		byzantine  string
		mods       []string
		randomTree bool
	}{
		{consensus: "chainedhotstuff", crypto: "ecdsa", byzantine: "", mods: nil},
		{consensus: "chainedhotstuff", crypto: "eddsa", byzantine: "", mods: nil},
		{consensus: "chainedhotstuff", crypto: "bls12", byzantine: "", mods: nil},
		{consensus: "fasthotstuff", crypto: "ecdsa", byzantine: "", mods: nil},
		{consensus: "fasthotstuff", crypto: "eddsa", byzantine: "", mods: nil},
		{consensus: "fasthotstuff", crypto: "bls12", byzantine: "", mods: nil},
		{consensus: "simplehotstuff", crypto: "ecdsa", byzantine: "", mods: nil},
		{consensus: "simplehotstuff", crypto: "eddsa", byzantine: "", mods: nil},
		{consensus: "simplehotstuff", crypto: "bls12", byzantine: "", mods: nil},
		{consensus: "chainedhotstuff", crypto: "ecdsa", byzantine: "fork:1", mods: nil},
		{consensus: "chainedhotstuff", crypto: "ecdsa", byzantine: "silence:1", mods: nil},
		{consensus: "chainedhotstuff", crypto: "ecdsa", byzantine: "", mods: []string{"kauri"}},
		{consensus: "chainedhotstuff", crypto: "bls12", byzantine: "", mods: []string{"kauri"}},
		{consensus: "chainedhotstuff", crypto: "ecdsa", byzantine: "", mods: []string{"kauri"}, randomTree: true},
		{consensus: "chainedhotstuff", crypto: "bls12", byzantine: "", mods: []string{"kauri"}, randomTree: true},
	}

	for _, tt := range tests {
		t.Run(test.Name([]string{"consensus", "crypto", "byzantine", "mods"}, tt.consensus, tt.crypto, tt.byzantine, tt.mods), func(t *testing.T) {
			var replicaOpts *orchestrationpb.ReplicaOpts
			if slices.Contains(tt.mods, "kauri") {
				replicaOpts = makeTreeReplicaOpts(tt.consensus, tt.crypto, tt.mods, 7, 2, tt.randomTree)
			} else {
				replicaOpts = makeReplicaOpts(tt.consensus, tt.crypto, tt.byzantine, tt.mods)
			}
			run(t, replicaOpts)
		})
	}
}

func TestDeployment(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" && runtime.GOOS != "linux" {
		t.Skip("GitHub Actions only supports linux containers on linux runners.")
	}

	clientOpts := &orchestrationpb.ClientOpts{
		ConnectTimeout: durationpb.New(time.Second),
		MaxConcurrent:  250,
		PayloadSize:    100,
		RateLimit:      math.Inf(1),
	}

	replicaOpts := &orchestrationpb.ReplicaOpts{
		BatchSize:         100,
		ConnectTimeout:    durationpb.New(time.Second),
		InitialTimeout:    durationpb.New(100 * time.Millisecond),
		TimeoutSamples:    1000,
		TimeoutMultiplier: 1.2,
		Consensus:         "chainedhotstuff",
		Crypto:            "ecdsa",
		LeaderRotation:    "round-robin",
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
		Replicas:     numReplicas,
		Clients:      numClients,
		ReplicaHosts: replicaHosts,
		ClientHosts:  clientHosts,
	}

	experiment, err := orchestration.NewExperiment(
		10*time.Second,
		"",
		replicaOpts,
		clientOpts,
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
