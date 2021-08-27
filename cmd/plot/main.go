package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/metrics/plotting"
	_ "github.com/relab/hotstuff/metrics/types"
)

var (
	interval            = flag.Duration("interval", time.Second, "Length of time interval to group measurements by.")
	latency             = flag.String("latency", "", "File to save latency plot to.")
	throughput          = flag.String("throughput", "", "File to save throughput plot to.")
	throughputVSLatency = flag.String("throughputvslatency", "", "File to save throughput vs latency plot to.")
)

func main() {
	flag.Parse()

	srcPath := flag.Arg(0)
	if srcPath == "" {
		fmt.Fprintf(os.Stderr, "usage: %s [flags] [path to measurements]\n", os.Args[0])
		os.Exit(1)
	}

	file, err := os.Open(srcPath)
	if err != nil {
		log.Fatalln(err)
	}

	latencyPlot := plotting.NewClientLatencyPlot()
	throughputPlot := plotting.NewThroughputPlot()
	throughputVSLatencyPlot := plotting.NewThroughputVSLatencyPlot()

	reader := plotting.NewReader(file, &latencyPlot, &throughputPlot, &throughputVSLatencyPlot)
	if err := reader.ReadAll(); err != nil {
		log.Fatalln(err)
	}

	if *latency != "" {
		if err := latencyPlot.PlotAverage(*latency, *interval); err != nil {
			log.Fatalln(err)
		}
	}

	if *throughput != "" {
		if err := throughputPlot.PlotAverage(*throughput, *interval); err != nil {
			log.Fatalln(err)
		}
	}

	if *throughputVSLatency != "" {
		if err := throughputVSLatencyPlot.PlotAverage(*throughputVSLatency, *interval); err != nil {
			log.Fatalln(err)
		}
	}
}
