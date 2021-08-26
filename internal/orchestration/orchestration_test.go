package orchestration_test

import (
	"net"
	"testing"
	"time"

	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/modules"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestOrchestration(t *testing.T) {
	run := func(consensusImpl string, crypto string) {
		controllerStream, workerStream := net.Pipe()

		workerProxy := orchestration.NewRemoteWorker(protostream.NewWriter(controllerStream), protostream.NewReader(controllerStream))
		worker := orchestration.NewWorker(protostream.NewWriter(workerStream), protostream.NewReader(workerStream), modules.NopLogger(), nil, 0)

		experiment := &orchestration.Experiment{
			NumReplicas: 4,
			NumClients:  2,
			ClientOpts: &orchestrationpb.ClientOpts{
				ConnectTimeout: durationpb.New(time.Second),
				MaxConcurrent:  250,
				PayloadSize:    100,
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
