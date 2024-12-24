// LatencyGen generates a Go source file containing the latency matrix.
package main

import (
	"embed"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/relab/hotstuff/internal/cli"
	"github.com/relab/hotstuff/internal/root"
)

//go:embed latencies/*.csv
var csvFiles embed.FS

//go:generate go run .

func main() {
	latencyFile := flag.String("file", "wonderproxy.csv", "csv file to use for latency matrix (default: wonderproxy)")
	flag.Parse()

	csvLatencies, err := csvFiles.ReadFile(filepath.Join("latencies", *latencyFile))
	if err != nil {
		log.Fatal(err)
	}
	allToAllMatrix, err := cli.ParseCSVLatencies(string(csvLatencies))
	if err != nil {
		log.Fatal(err)
	}
	latenciesGoCode, err := cli.GenerateGoLatencyMatrix(allToAllMatrix)
	if err != nil {
		log.Fatal(err)
	}

	// file path to save generated latencies to.
	dstFile := filepath.Join(root.Dir, "internal", "latency", "latency_matrix.go")
	if err = os.WriteFile(dstFile, latenciesGoCode, 0o600); err != nil {
		log.Fatal(err)
	}
}
