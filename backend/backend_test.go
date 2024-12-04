package backend

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/relab/hotstuff/modules"

	"github.com/golang/mock/gomock"
	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func TestConnect(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		const n = 4
		ctrl := gomock.NewController(t)
		td := setup(t, ctrl, n)
		builder := modules.NewBuilder(1, td.keys[0])
		testutil.TestModules(t, ctrl, 1, td.keys[0], &builder)
		teardown := createServers(t, td, ctrl)
		defer teardown()
		td.builders.Build()

		cfg := NewConfig(td.creds, gorums.WithDialTimeout(time.Second))

		builder.Add(cfg)
		builder.Build()

		err := cfg.Connect(td.replicas)
		if err != nil {
			t.Error(err)
		}
	}
	runBoth(t, run)
}

// Mainly test initialization of scoped modules and how they depend on each other.
func TestConnectScoped(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		const n = 4
		ctrl := gomock.NewController(t)
		td := setup(t, ctrl, n)
		builder := modules.NewBuilder(1, td.keys[0])
		testutil.TestModulesScoped(t, ctrl, 1, td.keys[0], &builder, 4)
		teardown := createServers(t, td, ctrl)
		defer teardown()
		td.builders.Build()

		cfg := NewConfig(td.creds, gorums.WithDialTimeout(time.Second))

		builder.Add(cfg)
		builder.Build()

		err := cfg.Connect(td.replicas)
		if err != nil {
			t.Error(err)
		}
	}
	runBoth(t, run)
}

// testBase is a generic test for a unicast/multicast call
func testBase(t *testing.T, typ any, send func(modules.Configuration), handle eventloop.EventHandler, opts ...eventloop.HandlerOption) {
	run := func(t *testing.T, setup setupFunc) {
		const n = 4
		ctrl := gomock.NewController(t)
		td := setup(t, ctrl, n)

		serverTeardown := createServers(t, td, ctrl)
		defer serverTeardown()

		cfg := NewConfig(td.creds, gorums.WithDialTimeout(time.Second))
		td.builders[0].Add(cfg)
		hl := td.builders.Build()

		err := cfg.Connect(td.replicas)
		if err != nil {
			t.Fatal(err)
		}
		defer cfg.Close()

		ctx, cancel := context.WithCancel(context.Background())
		for _, hs := range hl[1:] {
			var (
				eventLoop    *eventloop.ScopedEventLoop
				synchronizer modules.Synchronizer
			)
			hs.Get(&eventLoop, &synchronizer)
			eventLoop.RegisterHandler(typ, handle, opts...)
			synchronizer.Start(ctx)
			go eventLoop.Run(ctx)
		}
		send(cfg)
		cancel()
	}
	runBoth(t, run)
}

func TestPropose(t *testing.T) {
	var wg sync.WaitGroup
	want := hotstuff.ProposeMsg{
		ID: 1,
		Block: hotstuff.NewBlock(
			hotstuff.GetGenesis().Hash(),
			hotstuff.NewQuorumCert(
				nil,
				0,
				hotstuff.NullPipe, // TODO: Verify if this code conflicts with pipelining
				hotstuff.GetGenesis().Hash()),
			"foo", 1, 1, 0,
		),
		Pipe: 0,
	}

	testBase(t, want, func(cfg modules.Configuration) {
		wg.Add(3)
		cfg.Propose(want)
		wg.Wait()
	}, func(event any) {
		got := event.(hotstuff.ProposeMsg)
		if got.ID != want.ID {
			t.Errorf("wrong id in proposal: got: %d, want: %d", got.ID, want.ID)
		}
		if got.Block.Hash() != want.Block.Hash() {
			t.Error("block hashes do not match")
		}
		wg.Done()
	})
}

func TestProposeScoped(t *testing.T) {
	var wg sync.WaitGroup
	pipe := hotstuff.Pipe(123)
	want := hotstuff.ProposeMsg{
		ID: 1,
		Block: hotstuff.NewBlock(
			hotstuff.GetGenesis().Hash(),
			hotstuff.NewQuorumCert(
				nil,
				0,
				hotstuff.NullPipe, // TODO: Verify if this code conflicts with pipelining
				hotstuff.GetGenesis().Hash()),
			"foo", 1, 1, pipe,
		),
		Pipe: pipe,
	}

	testBase(t, want, func(cfg modules.Configuration) {
		wg.Add(3)
		cfg.Propose(want)
		wg.Wait()
	}, func(event any) {
		got := event.(hotstuff.ProposeMsg)
		if got.ID != want.ID {
			t.Errorf("wrong id in proposal: got: %d, want: %d", got.ID, want.ID)
		}
		if got.Block.Hash() != want.Block.Hash() {

			t.Errorf("block hashes do not match. want %d got %d", got.Block.Pipe(), want.Block.Pipe())
		}
		wg.Done()
	}, eventloop.RespondToScope(pipe))
}

func TestTimeout(t *testing.T) {
	var wg sync.WaitGroup
	want := hotstuff.TimeoutMsg{
		ID:            1,
		View:          1,
		ViewSignature: nil,
		SyncInfo:      hotstuff.NewSyncInfo(hotstuff.NullPipe),
	}
	testBase(t, want, func(cfg modules.Configuration) {
		wg.Add(3)
		cfg.Timeout(want)
		wg.Wait()
	}, func(event any) {
		got := event.(hotstuff.TimeoutMsg)
		if got.ID != want.ID {
			t.Errorf("wrong id in proposal: got: %d, want: %d", got.ID, want.ID)
		}
		if got.View != want.View {
			t.Errorf("wrong view in proposal: got: %d, want: %d", got.View, want.View)
		}
		wg.Done()
	})
}

func TestTimeoutScoped(t *testing.T) {
	var wg sync.WaitGroup

	pipe := hotstuff.Pipe(1)
	want := hotstuff.TimeoutMsg{
		ID:            1,
		View:          1,
		ViewSignature: nil,
		SyncInfo:      hotstuff.NewSyncInfo(pipe),
		Pipe:          pipe,
	}
	testBase(t, want, func(cfg modules.Configuration) {
		wg.Add(3)
		cfg.Timeout(want)
		wg.Wait()
	}, func(event any) {
		got := event.(hotstuff.TimeoutMsg)
		if got.ID != want.ID {
			t.Errorf("wrong id in proposal: got: %d, want: %d", got.ID, want.ID)
		}
		if got.View != want.View {
			t.Errorf("wrong view in proposal: got: %d, want: %d", got.View, want.View)
		}
		wg.Done()
	}, eventloop.RespondToScope(pipe))
}

type testData struct {
	n         int
	creds     credentials.TransportCredentials
	replicas  []ReplicaInfo
	listeners []net.Listener
	keys      []hotstuff.PrivateKey
	builders  testutil.BuilderList
}

type setupFunc func(t *testing.T, ctrl *gomock.Controller, n int) testData

func setupReplicas(t *testing.T, ctrl *gomock.Controller, n int) testData {
	t.Helper()

	listeners := make([]net.Listener, n)
	keys := make([]hotstuff.PrivateKey, 0, n)
	replicas := make([]ReplicaInfo, 0, n)

	// generate keys and replicaInfo
	for i := 0; i < n; i++ {
		listeners[i] = testutil.CreateTCPListener(t)
		keys = append(keys, testutil.GenerateECDSAKey(t))
		replicas = append(replicas, ReplicaInfo{
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
		builders:  testutil.CreateBuilders(t, ctrl, n, keys...),
	}
}

func setupTLS(t *testing.T, ctrl *gomock.Controller, n int) testData {
	t.Helper()
	td := setupReplicas(t, ctrl, n)

	certificates := make([]*x509.Certificate, 0, n)

	caPK := testutil.GenerateECDSAKey(t)
	ca, err := keygen.GenerateRootCert(caPK.(*ecdsa.PrivateKey))
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	for i := 0; i < n; i++ {
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

func createServers(t *testing.T, td testData, _ *gomock.Controller) (teardown func()) {
	t.Helper()
	servers := make([]*Server, td.n)
	for i := range servers {
		servers[i] = NewServer(WithGorumsServerOptions(gorums.WithGRPCServerOptions(grpc.Creds(td.creds))))
		servers[i].StartOnListener(td.listeners[i])
		td.builders[i].Add(servers[i])
	}
	return func() {
		for _, srv := range servers {
			srv.Stop()
		}
	}
}
