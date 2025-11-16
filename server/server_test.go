// Package server_test contains integration tests for the HotStuff server implementation.
// Tests verify network communication, proposal handling, and timeout mechanisms across
// multiple replicas with both TLS and non-TLS configurations.
package server_test

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/network"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/protocol/synchronizer"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto"
	"github.com/relab/hotstuff/security/crypto/keygen"
	"github.com/relab/hotstuff/server"
	"github.com/relab/hotstuff/wiring"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	// defaultReplicaCount is the standard number of replicas used in tests.
	defaultReplicaCount = 4
	// testTimeout is the maximum time to wait for expected events before failing a test.
	testTimeout = 5 * time.Second
)

type replicaDeps struct {
	wiring.Core
	wiring.Security
	Sender       *network.GorumsSender
	Server       *server.Server
	Synchronizer *synchronizer.Synchronizer
}

func TestConnect(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		td := setup(t, defaultReplicaCount)
		deps := createServers(t, td)
		first := deps[0]
		err := first.Sender.Connect(td.replicas)
		if err != nil {
			t.Error(err)
		}
	}
	runBoth(t, run)
}

type sendFunc func(*cert.Authority, *network.GorumsSender)

// testBase is a test helper that collects events for verification. The handle function is called
// for each received event and signals receipt on the events channel which is used to ensure that
// the expected number of events are received.
func testBase[T any](t *testing.T, n int, expectedEventsPerReplica int, send sendFunc, handle func(T)) {
	run := func(t *testing.T, setup setupFunc) {
		td := setup(t, n)
		deps := createServers(t, td)
		for _, dep := range deps {
			err := dep.Sender.Connect(td.replicas)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(dep.Sender.Close)
		}

		// Calculate total expected events across all receiving replicas (sender is excluded)
		expectedEvents := expectedEventsPerReplica * (n - 1)
		// Create event channel for this test run
		eventsCh := make(chan struct{}, expectedEvents)

		// Start all replicas with event handling and event handled notification
		ctx := t.Context()
		for _, d := range deps {
			eventloop.Register(d.EventLoop(), func(event T) {
				handle(event)
				eventsCh <- struct{}{} // event handled
			})
			d.Synchronizer.Start(ctx)
			go d.EventLoop().Run(ctx)
		}
		send(deps[0].Authority(), deps[0].Sender)

		for i := range expectedEvents {
			select {
			case <-eventsCh:
			case <-time.After(testTimeout):
				t.Fatalf("timeout waiting for events: received %d/%d", i, expectedEvents)
			}
		}
	}
	runBoth(t, run)
}

func TestPropose(t *testing.T) {
	// We expect to receive 1 ProposeMsg event per replica (4 replicas - 1 sender = 3 receivers)
	const expectedEventsPerReplica = 1

	var want hotstuff.ProposeMsg
	testBase(t, defaultReplicaCount, expectedEventsPerReplica,
		func(auth *cert.Authority, sender *network.GorumsSender) {
			// write the wanted test data to the variable in outer scope
			want = hotstuff.ProposeMsg{
				ID:    1,
				Block: testutil.CreateBlock(t, auth),
			}
			sender.Propose(&want)
		}, func(got hotstuff.ProposeMsg) {
			// This should be invoked 3 times (1 per replica, 3 receiving replicas)
			if got.ID != want.ID {
				t.Errorf("wrong id in proposal: got: %d, want: %d", got.ID, want.ID)
			}
			if got.Block.Hash() != want.Block.Hash() {
				t.Error("block hashes do not match")
			}
		})
}

func TestTimeout(t *testing.T) {
	// We expect to receive 2 TimeoutMsg events per replica (4 replicas - 1 sender = 3 receivers):
	// 1. The timeout sent explicitly by the sender
	// 2. An additional timeout triggered by the synchronizer's timer
	const expectedEventsPerReplica = 2

	view := hotstuff.View(1)
	want := hotstuff.TimeoutMsg{
		ID:       1,
		View:     view,
		SyncInfo: hotstuff.NewSyncInfo(),
	}
	testBase(t, defaultReplicaCount, expectedEventsPerReplica,
		func(auth *cert.Authority, sender *network.GorumsSender) {
			sig, err := auth.Sign(view.ToBytes())
			if err != nil {
				t.Fatal(err)
			}
			want.ViewSignature = sig
			// We send only a single timeout message, but the synchronizer triggers
			// an additional timeout, resulting from the following call chain:
			// Start() -> startTimeoutTimer() -> TimeoutEvent -> OnLocalTimeout() -> sender.Timeout()
			sender.Timeout(want)
		},
		func(got hotstuff.TimeoutMsg) {
			// This should be invoked 6 times (2 per replica, 3 receiving replicas)
			if got.ID != want.ID {
				t.Errorf("wrong id: got: %d, want: %d", got.ID, want.ID)
			}
			if got.View != want.View {
				t.Errorf("wrong view: got: %d, want: %d", got.View, want.View)
			}
		},
	)
}

type testData struct {
	n         int
	creds     credentials.TransportCredentials
	replicas  []hotstuff.ReplicaInfo
	listeners []net.Listener
	keys      []hotstuff.PrivateKey
}

type setupFunc func(t *testing.T, n int) testData

func setupReplicas(t *testing.T, n int) testData {
	t.Helper()

	listeners := make([]net.Listener, n)
	keys := make([]hotstuff.PrivateKey, 0, n)
	replicas := make([]hotstuff.ReplicaInfo, 0, n)

	// generate keys and replicaInfo
	for i := range n {
		listeners[i] = testutil.CreateTCPListener(t)
		keys = append(keys, testutil.GenerateECDSAKey(t))
		replicas = append(replicas, hotstuff.ReplicaInfo{
			ID:      hotstuff.ID(i) + 1,
			Address: listeners[i].Addr().String(),
			PubKey:  keys[i].Public(),
		})
	}

	return testData{
		n:         n,
		creds:     nil,
		replicas:  replicas,
		listeners: listeners,
		keys:      keys,
	}
}

func setupTLS(t *testing.T, n int) testData {
	t.Helper()
	td := setupReplicas(t, n)

	certificates := make([]*x509.Certificate, 0, n)

	caPK := testutil.GenerateECDSAKey(t)
	ca, err := keygen.GenerateRootCert(caPK.(*ecdsa.PrivateKey))
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	for i := range n {
		cert, err := keygen.GenerateTLSCert(
			hotstuff.ID(i)+1,
			[]string{"localhost", "127.0.0.1"},
			ca,
			td.replicas[i].PubKey.(*ecdsa.PublicKey),
			caPK.(*ecdsa.PrivateKey),
		)
		if err != nil {
			t.Fatalf("Failed to generate certificate: %v", err)
		}
		certificates = append(certificates, cert)
	}

	cp := x509.NewCertPool()
	cp.AddCert(ca)
	creds := credentials.NewTLS(&tls.Config{
		RootCAs:      cp,
		ClientCAs:    cp,
		Certificates: []tls.Certificate{{Certificate: [][]byte{certificates[0].Raw}, PrivateKey: td.keys[0]}},
		ClientAuth:   tls.RequireAndVerifyClientCert,
	})

	td.creds = creds
	return td
}

func runBoth(t *testing.T, run func(*testing.T, setupFunc)) {
	t.Helper()
	t.Run("NoTLS", func(t *testing.T) { run(t, setupReplicas) })
	t.Run("WithTLS", func(t *testing.T) { run(t, setupTLS) })
}

func createServers(t *testing.T, td testData) []replicaDeps {
	t.Helper()
	deps := make([]replicaDeps, 0, td.n)
	for i := range td.n {
		depsCore := wiring.NewCore(hotstuff.ID(i+1), "test", td.keys[i])
		sender := network.NewGorumsSender(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			td.creds,
		)
		depsSecurity := wiring.NewSecurity(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			sender,
			crypto.NewECDSA(depsCore.RuntimeCfg()),
		)
		server := server.NewServer(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			depsSecurity.Blockchain(),
			server.WithGorumsServerOptions(gorums.WithGRPCServerOptions(grpc.Creds(td.creds))),
		)
		server.StartOnListener(td.listeners[i])
		commandCache := clientpb.NewCommandCache(1)
		states, err := protocol.NewViewStates(
			depsSecurity.Blockchain(),
			depsSecurity.Authority(),
		)
		if err != nil {
			t.Fatal(err)
		}
		leaderRotation := leaderrotation.NewFixed(hotstuff.ID(1))
		depsConsensus := wiring.NewConsensus(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			depsSecurity.Blockchain(),
			depsSecurity.Authority(),
			commandCache,
			rules.NewChainedHotStuff(
				depsCore.Logger(),
				depsCore.RuntimeCfg(),
				depsSecurity.Blockchain(),
			),
			leaderRotation,
			states,
			comm.NewClique(
				depsCore.RuntimeCfg(),
				votingmachine.New(
					depsCore.Logger(),
					depsCore.EventLoop(),
					depsCore.RuntimeCfg(),
					depsSecurity.Blockchain(),
					depsSecurity.Authority(),
					states,
				),
				leaderRotation,
				sender,
			),
		)
		synchronizer := synchronizer.New(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			depsSecurity.Authority(),
			leaderRotation,
			synchronizer.NewFixedDuration(100*time.Millisecond),
			synchronizer.NewTimeoutRuler(depsCore.RuntimeCfg(), depsSecurity.Authority()),
			depsConsensus.Proposer(),
			depsConsensus.Voter(),
			states,
			sender,
		)
		deps = append(deps, replicaDeps{
			Core:         *depsCore,
			Security:     *depsSecurity,
			Sender:       sender,
			Server:       server,
			Synchronizer: synchronizer,
		})
	}
	t.Cleanup(func() {
		for _, d := range deps {
			d.Server.Stop()
		}
	})
	return deps
}
