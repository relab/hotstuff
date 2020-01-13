package main

import (
	"flag"
	"strings"
)

func main() {
	leader := flag.Bool("leader", false, "Designates this replica as leader")

	addresses := flag.String("nodes", "", "Semicolon separated list of addresses to connect to.")
	flag.Parse()

	nodes := strings.Split(*addresses, ";")

	if *leader {

	} else {

	}
}
