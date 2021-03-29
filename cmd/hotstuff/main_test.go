package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto"
)

func TestHotStuff(t *testing.T) {
	testdir, err := os.MkdirTemp("", "hotstufftest")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := os.RemoveAll(testdir)
		if err != nil {
			t.Log(err)
		}
	})

	ports := getFreePorts(t, 8)
	generateKeys(t, path.Join(testdir, "keys"))
	generateInput(t, path.Join(testdir, "input"))

	replicas := []replica{
		genReplica(testdir, 1, ports.next(), ports.next()),
		genReplica(testdir, 2, ports.next(), ports.next()),
		genReplica(testdir, 3, ports.next(), ports.next()),
		genReplica(testdir, 4, ports.next(), ports.next()),
	}

	clientConf := &options{
		SelfID:      1,
		Input:       path.Join(testdir, "input"),
		PayloadSize: 100,
		MaxInflight: 100,
		BatchSize:   10,
		Replicas:    replicas,
	}

	serverConf := &options{
		BatchSize:   10,
		PmType:      "round-robin",
		Replicas:    replicas,
		ViewTimeout: 100,
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := make(chan struct{}, len(replicas))
	for _, replica := range replicas {
		conf := *serverConf
		conf.SelfID = replica.ID
		conf.Privkey = fmt.Sprintf("%s/keys/%d.key", testdir, replica.ID)
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

	inputHash := hashFile(t, path.Join(testdir, "input"))
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

func generateKeys(t *testing.T, path string) {
	t.Helper()
	if err := crypto.GenerateConfiguration(path, true, 1, 4, "*", []string{"127.0.0.1"}); err != nil {
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
	_, err = io.CopyN(inputFile, rand.Reader, 1000)
	if err != nil {
		t.Fatal(err)
	}
}

func hashFile(t *testing.T, path string) (hash hotstuff.Hash) {
	t.Helper()
	f, err := os.Open(path)
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

func genReplica(testdir string, id hotstuff.ID, peerPort, clientPort int) replica {
	return replica{
		ID:         id,
		PeerAddr:   fmt.Sprintf("127.0.0.1:%d", peerPort),
		ClientAddr: fmt.Sprintf("127.0.0.1:%d", clientPort),
		Pubkey:     fmt.Sprintf("%s/keys/%d.key.pub", testdir, id),
		Cert:       fmt.Sprintf("%s/keys/%d.crt", testdir, id),
	}
}
