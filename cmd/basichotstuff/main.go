package main

import (
	"bufio"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/relab/hotstuff/pkg/basichotstuff"
)

func main() {
	leader := flag.Bool("leader", false, "Designates this replica as leader")
	addresses := flag.String("addresses", "", "Semicolon separated list of addresses for leader to connect to.")
	leaderAddr := flag.String("leader-address", "", "The address of the leader.")
	listenPort := flag.String("listen-port", "", "The port that a replica should listen to.")
	timeout := flag.Int("timeout", 5, "How many seconds a replica should wait before timing out.")
	commands := flag.String("commands", "", "File to read commands from.")
	flag.Parse()

	if *leader {
		replicas := strings.Split(*addresses, ";")
		// f = (n-1)/3, but n does not count the leader
		faultTolerance := len(replicas) / 3

		if faultTolerance < 1 {
			log.Fatalf("Need at least 4 replicas (including the leader) for the protocol to work.\n")
		}

		majority := len(replicas) + 1 - faultTolerance

		commandsChan := make(chan string, 10)
		file, err := os.Open(*commands)
		if err != nil {
			log.Fatalf("Failed to open commands file: %v", err)
		}
		scanner := bufio.NewScanner(file)
		go func() {
			for scanner.Scan() {
				commandsChan <- scanner.Text()
			}
		}()
		l := basichotstuff.NewLeader(basichotstuff.NewReplica(), majority, commandsChan)
		l.Init(replicas, *listenPort, time.Duration(*timeout)*time.Second)
	} else {
		r := basichotstuff.NewReplica()
		r.Init(*listenPort, *leaderAddr, time.Duration(*timeout)*time.Second)
	}
}
