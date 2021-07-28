package orchestration_test

import (
	"net"
	"testing"
	"time"

	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/modules"
)

func TestOrchestration(t *testing.T) {
	run := func(consensusImpl string, crypto string) {
		controllerStream, workerStream := net.Pipe()

		workerProxy := orchestration.NewRemoteWorker(protostream.NewWriter(controllerStream), protostream.NewReader(controllerStream))
		worker := orchestration.NewWorker(protostream.NewWriter(workerStream), protostream.NewReader(workerStream), modules.NopLogger())

		experiment := &orchestration.Experiment{
			NumReplicas:       4,
			NumClients:        2,
			BatchSize:         100,
			MaxConcurrent:     250,
			PayloadSize:       100,
			ConnectTimeout:    1 * time.Second,
			ViewTimeout:       1000,
			TimoutSamples:     1000,
			TimeoutMultiplier: 1.2,
			Duration:          1 * time.Second,
			Consensus:         consensusImpl,
			Crypto:            crypto,
			LeaderRotation:    "round-robin",
			Hosts:             map[string]orchestration.RemoteWorker{"127.0.0.1": workerProxy},
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
}
