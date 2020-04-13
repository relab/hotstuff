package gorumshotstuff

import (
	"crypto/ecdsa"
	"sync"
	"testing"
	"time"

	"github.com/relab/hotstuff"
)

// This test verifies that the entire stack works.
func TestHotStuff(t *testing.T) {
	keys := make(map[hotstuff.ReplicaID]*ecdsa.PrivateKey)
	keys[1], _ = hotstuff.GeneratePrivateKey()
	keys[2], _ = hotstuff.GeneratePrivateKey()
	keys[3], _ = hotstuff.GeneratePrivateKey()
	keys[4], _ = hotstuff.GeneratePrivateKey()

	config := hotstuff.NewConfig()
	config.Replicas[1] = &hotstuff.ReplicaInfo{ID: 1, Address: "127.0.0.1:13371", PubKey: &keys[1].PublicKey}
	config.Replicas[2] = &hotstuff.ReplicaInfo{ID: 2, Address: "127.0.0.1:13372", PubKey: &keys[2].PublicKey}
	config.Replicas[3] = &hotstuff.ReplicaInfo{ID: 3, Address: "127.0.0.1:13373", PubKey: &keys[3].PublicKey}
	config.Replicas[4] = &hotstuff.ReplicaInfo{ID: 4, Address: "127.0.0.1:13374", PubKey: &keys[4].PublicKey}

	out := make(map[hotstuff.ReplicaID]chan []hotstuff.Command)
	out[1] = make(chan []hotstuff.Command, 1)
	out[2] = make(chan []hotstuff.Command, 2)
	out[3] = make(chan []hotstuff.Command, 3)
	out[4] = make(chan []hotstuff.Command, 4)

	replicas := make(map[hotstuff.ReplicaID]*GorumsHotStuff)
	replicas[2] = New(10*time.Second, time.Second)
	replicas[1] = New(10*time.Second, time.Second)
	replicas[3] = New(10*time.Second, time.Second)
	replicas[4] = New(10*time.Second, time.Second)

	hotstuff.New(1, keys[1], config, replicas[1], 50*time.Millisecond, func(b []hotstuff.Command) { out[1] <- b })
	hotstuff.New(2, keys[2], config, replicas[2], 50*time.Millisecond, func(b []hotstuff.Command) { out[2] <- b })
	hotstuff.New(3, keys[3], config, replicas[3], 50*time.Millisecond, func(b []hotstuff.Command) { out[3] <- b })
	hotstuff.New(4, keys[4], config, replicas[4], 50*time.Millisecond, func(b []hotstuff.Command) { out[4] <- b })

	var wg sync.WaitGroup
	wg.Add(len(replicas))
	for id := range replicas {
		go func(id hotstuff.ReplicaID) {
			err := replicas[id].Start()
			if err != nil {
				t.Errorf("Failed to init replica %d: %v", id, err)
			}
			wg.Done()
		}(id)
	}

	wg.Wait()

	test := []hotstuff.Command{hotstuff.Command("DECIDE"), hotstuff.Command("COMMIT"), hotstuff.Command("PRECOMMIT"), hotstuff.Command("PROPOSE")}

	for _, r := range replicas {
		go func(r *hotstuff.HotStuff) {
			n := r.GetNotifier()
			for range n {
			}
		}(r.HotStuff)
	}

	for _, t := range test {
		replicas[1].HotStuff.AddCommand(t)
		replicas[1].HotStuff.Propose()
	}

	for id := range replicas {
		fail := false
		select {
		case o := <-out[id]:
			fail = o[0] != test[0]
		default:
			fail = true
		}
		if fail {
			t.Errorf("Replica %d: Incorrect output!", id)
		}
	}

	for _, replica := range replicas {
		replica.Close()
	}
}
