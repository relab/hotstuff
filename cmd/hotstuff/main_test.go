package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/keygen"
)

var (
	batchSize   int
	payloadSize int
	fileSize    int
	inFile      string
	outdir      string
)

func TestMain(m *testing.M) {
	flag.IntVar(&batchSize, "batch-size", 50, "The number of commands to batch together for each block.")
	flag.IntVar(&payloadSize, "payload-size", 10, "The size (in bytes) of each command.")
	flag.IntVar(&fileSize, "file-size", 100000, "The size (in bytes) of the input file.")
	flag.StringVar(&inFile, "input", "", "The input file. If empty, a random file will be generated based on 'file-size'.")
	flag.StringVar(&outdir, "outdir", "", "The path to the output directory. If empty, a temporary directory will be used.")
	flag.Parse()

	rc := m.Run()
	os.Exit(rc)
}

// TestSMR verifies that SMR works correctly. We run four replicas and a single client,
// and then feed a random input to the replicas. Afterwards, we compare each replica's output
// with the input to make sure that it got replicated correctly.
func TestSMR(t *testing.T) {
	t.Run("ECDSA", func(t *testing.T) { testSMRImpl(t, false) })
	t.Run("BLS12-381", func(t *testing.T) { testSMRImpl(t, true) })
}

func testSMRImpl(t *testing.T, bls bool) {
	var (
		testdir string
		input   string
	)

	if outdir != "" {
		testdir = outdir
	} else {
		testdir = t.TempDir()
	}

	if inFile != "" {
		input = inFile
	} else {
		input = path.Join(testdir, "input")
		generateInput(t, input)
	}

	ports := getFreePorts(t, 8)
	generateKeys(t, path.Join(testdir, "keys"), bls)

	replicas := []replica{
		genReplica(testdir, 1, ports.next(), ports.next(), bls),
		genReplica(testdir, 2, ports.next(), ports.next(), bls),
		genReplica(testdir, 3, ports.next(), ports.next(), bls),
		genReplica(testdir, 4, ports.next(), ports.next(), bls),
	}

	clientConf := &options{
		SelfID:      1,
		Input:       input,
		PayloadSize: payloadSize,
		MaxInflight: uint64(4 * batchSize),
		Replicas:    replicas,
		RootCAs:     []string{path.Join(testdir, "keys", "ca.crt")},
		TLS:         true,
	}

	serverConf := &options{
		BatchSize:   batchSize,
		PmType:      "round-robin",
		Replicas:    replicas,
		ViewTimeout: 100,
		RootCAs:     []string{path.Join(testdir, "keys", "ca.crt")},
		TLS:         true,
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := make(chan struct{}, len(replicas))
	for _, replica := range replicas {
		conf := *serverConf
		conf.SelfID = replica.ID
		conf.Cert = fmt.Sprintf("%s/keys/%d.crt", testdir, replica.ID)
		conf.CertKey = fmt.Sprintf("%s/keys/%d.key", testdir, replica.ID)
		if bls {
			conf.Crypto = "bls12"
			conf.Privkey = fmt.Sprintf("%s/keys/%d.bls", testdir, replica.ID)
		} else {
			conf.Crypto = "ecdsa"
			conf.Privkey = fmt.Sprintf("%s/keys/%d.key", testdir, replica.ID)
		}
		conf.Output = fmt.Sprintf("%s/%d.out", testdir, replica.ID)
		go func() {
			runServer(ctx, &conf)
			c <- struct{}{}
		}()
	}

	runClient(ctx, clientConf)
	cancel()

	// make sure all replicas get to stop and close their output files
	for range replicas {
		<-c
	}

	inputHash := hashFile(t, input)
	for _, replica := range replicas {
		outHash := hashFile(t, fmt.Sprintf("%s/%d.out", testdir, replica.ID))
		if inputHash != outHash {
			t.Error("Hash mismatch")
		}
	}
}

type ports []int

func (p *ports) next() int {
	port := (*p)[0]
	*p = (*p)[1:]
	return port
}

// getFreePorts will get free ports from the kernel by opening a listener on 127.0.0.1:0 and then closing it.
func getFreePorts(t *testing.T, n int) ports {
	ports := make(ports, n)
	for i := 0; i < n; i++ {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			err := lis.Close()
			if err != nil {
				t.Fatal(err)
			}
		}()
		ports[i] = lis.Addr().(*net.TCPAddr).Port
	}
	return ports
}

func generateKeys(t *testing.T, path string, bls bool) {
	t.Helper()
	if err := keygen.GenerateConfiguration(path, true, bls, 1, 4, "*", []string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
}

func generateInput(t *testing.T, path string) {
	t.Helper()
	inputFile, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	defer func() {
		err := inputFile.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()
	if err != nil {
		t.Fatal(err)
	}

	// create some input data to replicate
	_, err = io.CopyN(inputFile, rand.Reader, int64(fileSize))
	if err != nil {
		t.Fatal(err)
	}
}

func hashFile(t *testing.T, path string) (hash hotstuff.Hash) {
	t.Helper()
	f, err := os.Open(path)
	defer func() {
		err := f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	h.Sum(hash[:0])
	return hash
}

func genReplica(testdir string, id hotstuff.ID, peerPort, clientPort int, bls bool) replica {
	r := replica{
		ID:         id,
		PeerAddr:   fmt.Sprintf("127.0.0.1:%d", peerPort),
		ClientAddr: fmt.Sprintf("127.0.0.1:%d", clientPort),
		Cert:       fmt.Sprintf("%s/keys/%d.crt", testdir, id),
	}
	if bls {
		r.Pubkey = fmt.Sprintf("%s/keys/%d.bls.pub", testdir, id)
	} else {
		r.Pubkey = fmt.Sprintf("%s/keys/%d.key.pub", testdir, id)
	}
	return r
}
