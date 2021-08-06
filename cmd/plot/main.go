package main

import (
	"log"
	"os"
	"time"

	_ "github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/metrics/plotting"
	_ "github.com/relab/hotstuff/metrics/types"
)

func main() {
	file, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatalln(err)
	}
	latencyPlot := plotting.NewClientLatencyPlot()
	throughputPlot := plotting.NewThroughputPlot()
	reader := plotting.NewReader(file, &latencyPlot, &throughputPlot)
	if err := reader.ReadAll(); err != nil {
		log.Fatalln(err)
	}
	if err := latencyPlot.PlotAverage(os.Args[2], time.Second); err != nil {
		log.Fatalln(err)
	}
	if err := throughputPlot.PlotAverage(os.Args[3], time.Second); err != nil {
		log.Fatalln(err)
	}
}
