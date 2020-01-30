package main

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/relab/hotstuff/pkg/hotstuff"
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

func exec(cmd []byte) {
	s := string(cmd)
	log.Printf("Exec: %s\n", s)
}

func main() {
	pflag.Int("self-id", 0, "The ID for this replica")
	pflag.Int("leader-id", 0, "The ID of the fixed leader")
	pflag.Int("timeout", 1000, "Timeout (in milliseconds)")
	pflag.String("keyfile", "", "The path to the private key file")
	pflag.String("commands", "", "The file to read commands from")
	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	viper.SetConfigName("hotstuff_config")
	viper.SetConfigType("toml")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Failed to read config file: %v\n", err)
	}

	var replicas []map[string]string

	err = viper.UnmarshalKey("replicas", &replicas)
	if err != nil {
		log.Fatalf("Failed to unmarshal config file: %v\n", err)
	}

	config := hotstuff.NewConfig()

	selfID := hotstuff.ReplicaID(viper.GetInt("self-id"))
	leaderID := hotstuff.ReplicaID(viper.GetInt("leader-id"))
	keyFile := viper.GetString("keyfile")
	privKey, err := hotstuff.ReadPrivateKeyFile(keyFile)
	if err != nil {
		log.Fatalf("Failed to read private key file: %v\n", err)
	}

	selfPort := ""
	for _, replica := range replicas {
		pubKey, err := hotstuff.ReadPublicKeyFile(mapRead(replica, "keyfile"))
		if err != nil {
			log.Fatalf("Failed to read public key file %s: %v\n", mapRead(replica, "keyfile"), err)
		}
		socket := mapRead(replica, "socket")
		_id, err := strconv.Atoi(mapRead(replica, "id"))
		if err != nil {
			log.Fatalf("Failed to parse id. (Id must be an integer): %v\n", err)
		}
		id := hotstuff.ReplicaID(_id)
		// dont put self in config
		if id == selfID {
			selfPort = socket[strings.LastIndex(socket, ":")+1:]
		}
		info := &hotstuff.ReplicaInfo{
			ID:     id,
			Socket: socket,
			PubKey: pubKey,
		}
		config.Replicas[id] = info
	}

	if selfPort == "" {
		log.Fatalf("Found no port for self. Missing from config?\n")
	}

	timeout := time.Duration(viper.GetInt("timeout")) * time.Millisecond

	commands := make(chan []byte, 10)
	if selfID == leaderID {
		// send commands
		go func() {
			file, err := os.Open(viper.GetString("commands"))
			if err != nil {
				log.Fatalf("Failed to read commands file: %v\n", err)
			}
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				commands <- scanner.Bytes()
			}
		}()
	}

	pm := &hotstuff.FixedLeaderPacemaker{Leader: leaderID}
	hs := hotstuff.New(selfID, privKey, config, pm, timeout, commands, exec)
	pm.HS = hs
	err = hs.Init(selfPort)
	if err != nil {
		log.Fatalf("Failed to init HotStuff: %v\n", err)
	}
	pm.Start()
	log.Printf("Replica %d EXIT\n", selfID)
}
