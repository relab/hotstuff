package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/gorumshotstuff"
	"github.com/relab/hotstuff/pacemaker"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	mut         sync.Mutex
	lastExec    time.Time
	numCommands uint64
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
		commands <- batch
	}
}

func onExec(cmds []hotstuff.Command) {
	mut.Lock()
	defer mut.Unlock()
	now := time.Now()
	diff := now.Sub(lastExec)
	numCommands += uint64(len(cmds))
	if diff.Seconds() > 1.0 {
		lastExec = now
		throughput := (float64(numCommands) / 1000) / diff.Seconds()
		fmt.Printf("Throughput: %.2f Kops/sec\n", throughput)
		numCommands = 0
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
