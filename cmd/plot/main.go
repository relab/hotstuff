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
	reader := plotting.NewReader(file, &latencyPlot)
	if err := reader.ReadAll(); err != nil {
		log.Fatalln(err)
	}
	if err := latencyPlot.PlotAverage(os.Args[2], time.Second); err != nil {
		log.Fatalln(err)
	}
}
