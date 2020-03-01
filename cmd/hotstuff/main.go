package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/pprof"
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
	fmt.Print(s)
}

func main() {
	pflag.Int("self-id", 0, "The ID for this replica")
	pflag.Int("leader-id", 0, "The ID of the fixed leader")
	pflag.Int("timeout", 1000, "Timeout (in milliseconds)")
	pflag.String("keyfile", "", "The path to the private key file")
	pflag.String("commands", "", "The file to read commands from")
	pflag.String("cpuprofile", "", "File to write CPU profile to")
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
		address := mapRead(replica, "address")
		_id, err := strconv.Atoi(mapRead(replica, "id"))
		if err != nil {
			log.Fatalf("Failed to parse id. (Id must be an integer): %v\n", err)
		}
		id := hotstuff.ReplicaID(_id)
		// dont put self in config
		if id == selfID {
			selfPort = address[strings.LastIndex(address, ":")+1:]
		}
		info := &hotstuff.ReplicaInfo{
			ID:      id,
			Address: address,
			PubKey:  pubKey,
		}
		config.Replicas[id] = info
	}

	if selfPort == "" {
		log.Fatalf("Found no port for self. Missing from config?\n")
	}

	timeout := time.Duration(viper.GetInt("timeout")) * time.Millisecond

	// send commands
	commands := make(chan []byte, 10)
	if commandsFile := viper.GetString("commands"); commandsFile != "" {
		go func() {
			file, err := os.Open(commandsFile)
			if err != nil {
				log.Fatalf("Failed to read commands file: %v\n", err)
			}
			reader := bufio.NewReader(file)
			for {
				buf := make([]byte, 1024)
				_, err := io.ReadFull(reader, buf)
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					break
				} else if err != nil {
					log.Fatalf("Failed to read from file: %v", err)
				}
				commands <- buf
			}
			// send some extra commands such that the last part of the file will be committed successfully
			commands <- []byte{}
			commands <- []byte{}
			commands <- []byte{}
			close(commands)
		}()
	}

	var pm hotstuff.Pacemaker
	hs := hotstuff.New(selfID, privKey, config, nil, timeout, exec)

	pmType := viper.GetString("pacemaker")
	switch pmType {
	case "RR":
		s := viper.GetIntSlice("leaderSchedule")
		var leaderSchedule []hotstuff.ReplicaID
		for _, id := range s {
			leaderSchedule = append(leaderSchedule, hotstuff.ReplicaID(id))
		}
		termLength := viper.GetInt("termLength")
		pm = &hotstuff.RoundRobinPacemaker{TermLength: termLength, Schedule: leaderSchedule, HS: hs, Commands: commands}
	case "fixed":
		pm = &hotstuff.FixedLeaderPacemaker{HS: hs, Leader: leaderID, Commands: commands}
	}

	hs.Pacemaker = pm
	err = hs.Init(selfPort)
	if err != nil {
		log.Fatalf("Failed to init HotStuff: %v\n", err)
	}

	if cpuprofile := viper.GetString("cpuprofile"); cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			log.Fatal("Could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("Could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}
	pm.Run()
	log.Printf("Replica %d EXIT\n", selfID)
}
