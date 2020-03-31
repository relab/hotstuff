package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/gorumshotstuff"
	"github.com/relab/hotstuff/pacemaker"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	backlog chan hotstuff.Command
	done    chan uint32

	lastExec     int64
	numCommands  uint64
	latencyTotal int64
	sentTimes    map[uint32]int64 = make(map[uint32]int64)
	mut          sync.Mutex
)

// helper function to ensure that we dont try to read values that dont exist
func mapRead(m map[string]string, key string) string {
	v, ok := m[key]
	if !ok {
		log.Fatalf("Missing value for '%s',\n", key)
	}
	return v
}

func createCommands() {
	var num uint32 = 0
	for {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, num)
		cmd := hotstuff.Command(b)
		backlog <- cmd
		time := time.Now().UnixNano()
		mut.Lock()
		sentTimes[num] = time
		mut.Unlock()
		num++
	}
}

func sendCommands(commands chan<- []hotstuff.Command, batchSize int) {
	for {
		batch := make([]hotstuff.Command, batchSize)
		for i := 0; i < batchSize; i++ {
			var ok bool
			batch[i], ok = <-backlog
			if !ok {
				return
			}
		}
		commands <- batch
	}
}

func onExec(cmds []hotstuff.Command) {
	now := time.Now().UnixNano()
	num := atomic.AddUint64(&numCommands, uint64(len(cmds)))
	last := atomic.LoadInt64(&lastExec)
	diff := now - last

	// compute latencies
	var latency int64
	mut.Lock()
	for _, c := range cmds {
		if len(c) == 4 {
			id := binary.LittleEndian.Uint32(c)
			latency += now - sentTimes[id]
			delete(sentTimes, id)
		}
	}
	mut.Unlock()

	latency = atomic.AddInt64(&latencyTotal, latency)

	if seconds := float64(diff) / float64(time.Second); seconds > 1.0 {
		atomic.StoreInt64(&lastExec, now)
		// compute throughput
		throughput := (float64(num) / 1000) / seconds
		// compute latency
		latencyAvg := (float64(latency) / float64(num)) / float64(time.Millisecond)
		fmt.Printf("Throughput: %.2f Kops/sec, Avg. Latency: %.2f ms\n", throughput, latencyAvg)
		atomic.StoreUint64(&numCommands, 0)
		atomic.StoreInt64(&latencyTotal, 0)
	}
}

func main() {
	signals := make(chan os.Signal)
	signal.Notify(signals, os.Interrupt)

	selfID := pflag.Int("self-id", 0, "The ID for this replica")
	keyFile := pflag.String("keyfile", "", "The path to the private key file")
	configFile := pflag.String("config", "", "The path to the config file")
	batchSize := pflag.Int("batch-size", 100, "The size of batches")
	pflag.Parse()

	viper.SetConfigFile(*configFile)
	viper.SetConfigType("toml")
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Failed to read config file: %v\n", err)
	}

	backlog = make(chan hotstuff.Command, 2**batchSize)
	done = make(chan uint32, 2**batchSize)

	privKey, err := hotstuff.ReadPrivateKeyFile(*keyFile)
	if err != nil {
		log.Fatalf("Failed to read private key file: %v\n", err)
	}

	var replicas []map[string]string
	err = viper.UnmarshalKey("replicas", &replicas)
	if err != nil {
		log.Fatalf("Failed to unmarshal config file: %v\n", err)
	}

	config := hotstuff.NewConfig()
	for _, replica := range replicas {
		id, err := strconv.Atoi(mapRead(replica, "id"))
		if err != nil {
			log.Fatalf("Failed to read id: %v\n", err)
		}
		addr := mapRead(replica, "address")
		pubkey, err := hotstuff.ReadPublicKeyFile(mapRead(replica, "keyfile"))
		if err != nil {
			log.Fatalf("Failed to read public key file: %v\n", err)
		}
		config.Replicas[hotstuff.ReplicaID(id)] = &hotstuff.ReplicaInfo{
			ID:      hotstuff.ReplicaID(id),
			Address: addr,
			PubKey:  pubkey,
		}
	}

	commands := make(chan []hotstuff.Command, 10)
	if *selfID == viper.GetInt("leader_id") {
		go createCommands()
		go sendCommands(commands, *batchSize)
	}

	backend := gorumshotstuff.New(time.Minute, time.Second)
	hs := hotstuff.New(hotstuff.ReplicaID(*selfID), privKey, config, backend, 500*time.Millisecond, onExec)
	pm := &pacemaker.FixedLeaderPacemaker{
		Leader:   hotstuff.ReplicaID(viper.GetInt("leader_id")),
		Commands: commands,
		HotStuff: hs,
	}

	err = backend.Start()
	if err != nil {
		log.Fatalf("Failed to start backend: %v\n", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go pm.Run(ctx)

	<-signals
	cancel()
}
