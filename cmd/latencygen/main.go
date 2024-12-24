// LatencyGen generates a Go source file containing the latency matrix.
package main

import (
	_ "embed"
	"flag"
	"log"
	"os"

	"github.com/relab/hotstuff/internal/cli"
)

//go:embed latencies.csv
var csvLatencies string

//go:generate go run .

func main() {
	dstFile := flag.String("dest", "../../internal/latency/latency_matrix.go", "file path to save latencies to.")
	flag.Parse()
	if *dstFile == "" {
		flag.Usage()
		os.Exit(1)
	}
	allToAllMatrix, err := cli.ParseCSVLatencies(csvLatencies)
	if err != nil {
		log.Fatal(err)
	}
	latenciesGoCode, err := cli.GenerateGoLatencyMatrix(allToAllMatrix)
	if err != nil {
		log.Fatal(err)
	}
	if err = os.WriteFile(*dstFile, latenciesGoCode, 0o600); err != nil {
		log.Fatal(err)
	}
}
