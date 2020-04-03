package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/gorumshotstuff"
	"github.com/relab/hotstuff/pacemaker"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// helper function to ensure that we dont try to read values that dont exist
func mapRead(m map[string]string, key string) string {
	v, ok := m[key]
	if !ok {
		log.Fatalf("Missing value for '%s',\n", key)
	}
	return v
}

func sendCommands(commands chan<- []hotstuff.Command, batchSize int) {
	for {
		batch := make([]hotstuff.Command, batchSize)
		for i := 0; i < batchSize; i++ {
			now := time.Now().UnixNano()
			b := make([]byte, binary.MaxVarintLen64)
			len := binary.PutVarint(b, now)
			if len > 0 {
				batch[i] = hotstuff.Command(b[:len])
			}
		}
		commands <- batch
	}
}

func processCommands(cmdsChan <-chan []hotstuff.Command) {
	var (
		lastExec     int64
		numCommands  int
		latencyTotal int64
	)

	for cmds := range cmdsChan {
		// compute latencies
		now := time.Now().UnixNano()
		for _, c := range cmds {
			sent, n := binary.Varint(c)
			if n > 0 {
				latencyTotal += now - sent
			}
		}

		numCommands += len(cmds)
		diff := now - lastExec
		if seconds := float64(diff) / float64(time.Second); seconds > 1 {
			lastExec = now
			// compute throughput
			throughput := (float64(numCommands) / 1000) / seconds
			// compute latency
			latencyAvg := (float64(latencyTotal) / float64(numCommands)) / float64(time.Millisecond)
			fmt.Printf("Throughput: %.2f Kops/sec, Avg. Latency: %.2f ms\n", throughput, latencyAvg)
			numCommands = 0
			latencyTotal = 0
		}
	}
}

func main() {
	signals := make(chan os.Signal)
	signal.Notify(signals, os.Interrupt)

	selfID := pflag.Int("self-id", 0, "The ID for this replica")
	keyFile := pflag.String("keyfile", "", "The path to the private key file")
	configFile := pflag.String("config", "", "The path to the config file")
	batchSize := pflag.Int("batch-size", 100, "The size of batches")
	cpuprofile := pflag.String("cpuprofile", "", "The file to write a cpuprofile to")
	pflag.Parse()

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("Could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("Could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	viper.SetConfigFile(*configFile)
	viper.SetConfigType("toml")
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Failed to read config file: %v\n", err)
	}

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
		go sendCommands(commands, *batchSize)
	}

	exec := make(chan []hotstuff.Command, 10)
	go processCommands(exec)

	backend := gorumshotstuff.New(time.Minute, time.Second)

	hs := hotstuff.New(hotstuff.ReplicaID(*selfID), privKey, config, backend, 500*time.Millisecond,
		func(cmds []hotstuff.Command) {
			exec <- cmds
		},
	)

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
	backend.Close()
}
