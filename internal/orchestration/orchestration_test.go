package orchestration_test

import (
	"net"
	"testing"
	"time"

	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/protostream"
)

func TestOrchestration(t *testing.T) {
	controllerStream, workerStream := net.Pipe()

	workerProxy := orchestration.NewRemoteWorker(protostream.NewWriter(controllerStream), protostream.NewReader(controllerStream))
	worker := orchestration.NewWorker(protostream.NewWriter(workerStream), protostream.NewReader(workerStream))

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
		Consensus:         "chainedhotstuff",
		Crypto:            "ecdsa",
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
