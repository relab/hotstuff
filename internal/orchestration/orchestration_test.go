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

	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/iago/iagotest"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestOrchestration(t *testing.T) {
	run := func(consensusImpl string, crypto string) {
		controllerStream, workerStream := net.Pipe()

		workerProxy := orchestration.NewRemoteWorker(protostream.NewWriter(controllerStream), protostream.NewReader(controllerStream))
		worker := orchestration.NewWorker(protostream.NewWriter(workerStream), protostream.NewReader(workerStream), modules.NopLogger(), nil, 0)

		experiment := &orchestration.Experiment{
			Logger:      logging.New("ctrl"),
			NumReplicas: 4,
			NumClients:  2,
			ClientOpts: &orchestrationpb.ClientOpts{
				ConnectTimeout: durationpb.New(time.Second),
				MaxConcurrent:  250,
				PayloadSize:    100,
				RateLimit:      math.Inf(1),
			},
			ReplicaOpts: &orchestrationpb.ReplicaOpts{
				BatchSize:         100,
				ConnectTimeout:    durationpb.New(time.Second),
				InitialTimeout:    durationpb.New(100 * time.Millisecond),
				TimeoutSamples:    1000,
				TimeoutMultiplier: 1.2,
				Consensus:         consensusImpl,
				Crypto:            crypto,
				LeaderRotation:    "round-robin",
			},
			Duration: 1 * time.Second,
			Hosts:    map[string]orchestration.RemoteWorker{"127.0.0.1": workerProxy},
		}

		c := make(chan error)
		go func() {
			c <- worker.Run()
		}()

		err := experiment.Run()
		if err != nil {
			t.Fatal(err)
		}

		err = <-c
		if err != nil {
			t.Fatal(err)
		}
	}

	t.Run("ChainedHotStuff+ECDSA", func(t *testing.T) { run("chainedhotstuff", "ecdsa") })
	t.Run("ChainedHotStuff+BLS12", func(t *testing.T) { run("chainedhotstuff", "bls12") })
	t.Run("Fast-HotStuff+ECDSA", func(t *testing.T) { run("fasthotstuff", "ecdsa") })
	t.Run("Fast-HotStuff+BLS12", func(t *testing.T) { run("fasthotstuff", "bls12") })
	t.Run("Simple-HotStuff+ECDSA", func(t *testing.T) { run("simplehotstuff", "ecdsa") })
	t.Run("Simple-HotStuff+BLS12", func(t *testing.T) { run("simplehotstuff", "bls12") })
}

func TestDeployment(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" && runtime.GOOS != "linux" {
		t.Skip("GitHub Actions only supports linux containers on linux runners.")
	}

	experiment := &orchestration.Experiment{
		Logger:      logging.New("ctrl"),
		NumReplicas: 4,
		NumClients:  2,
		ClientOpts: &orchestrationpb.ClientOpts{
			ConnectTimeout: durationpb.New(time.Second),
			MaxConcurrent:  250,
			PayloadSize:    100,
			RateLimit:      math.Inf(1),
		},
		ReplicaOpts: &orchestrationpb.ReplicaOpts{
			BatchSize:         100,
			ConnectTimeout:    durationpb.New(time.Second),
			InitialTimeout:    durationpb.New(100 * time.Millisecond),
			TimeoutSamples:    1000,
			TimeoutMultiplier: 1.2,
			Consensus:         "chainedhotstuff",
			Crypto:            "ecdsa",
			LeaderRotation:    "round-robin",
		},
		Duration: 1 * time.Second,
		Hosts:    make(map[string]orchestration.RemoteWorker),
	}

	exe := compileBinary(t)
	g := iagotest.CreateSSHGroup(t, 4, true)

	sessions, err := orchestration.Deploy(g, orchestration.DeployConfig{
		ExePath:  exe,
		LogLevel: "info",
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(len(sessions))
	for host, session := range sessions {
		experiment.Hosts[host] = orchestration.NewRemoteWorker(protostream.NewWriter(session.Stdin()), protostream.NewReader(session.Stdout()))
		go func(session orchestration.WorkerSession) {
			_, err := io.Copy(os.Stderr, session.Stderr())
			if err != nil {
				t.Error("failed to copy stderr:", err)
			}
			wg.Done()
		}(session)
	}
	err = experiment.Run()
	if err != nil {
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
