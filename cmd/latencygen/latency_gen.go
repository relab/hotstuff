// LatencyGen generates a Go source file containing the latency matrix.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/exp/maps"
)

// Run go generate from this directory to generate the latency matrix.
//
//go:generate go run . -dest ../../backend/latencies.go
func main() {
	dstFile := flag.String("dest", "", "Destination path and file to save latency map file to.")
	flag.Parse()
	if *dstFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	var allToAllMatrix map[string]map[string]string
	if err := json.Unmarshal([]byte(latencyMatrix), &allToAllMatrix); err != nil {
		log.Fatal(err)
	}
	keys := maps.Keys(allToAllMatrix)
	sort.Strings(keys)

	s := strings.Builder{}
	s.WriteString(`package backend

import "time"

var latencies = map[string]map[string]time.Duration{
`)
	for _, city := range keys {
		s.WriteString(fmt.Sprintf("%q: {\n", city))
		latencies := allToAllMatrix[city]
		for _, city2 := range keys {
			latency, err := time.ParseDuration(latencies[city2] + "ms")
			if err != nil {
				log.Fatal(err)
			}
			s.WriteString(fmt.Sprintf("%q: %d,\n", city2, int64(latency)))
		}
		s.WriteString(fmt.Sprintln("},"))
	}
	s.WriteString(fmt.Sprintln("}"))
	latenciesGoCode, err := format.Source([]byte(s.String()))
	if err != nil {
		log.Fatal(err)
	}
	err = os.WriteFile(*dstFile, latenciesGoCode, 0o600)
	if err != nil {
		log.Fatal(err)
	}
}
