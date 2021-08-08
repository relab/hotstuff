package plotting

import (
	"fmt"
	"image/color"
	"time"

	"github.com/relab/hotstuff/metrics/types"
	"go-hep.org/x/hep/hplot"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

// ThroughputVSLatencyPlot is a plotter that plots throughput vs time.
type ThroughputVSLatencyPlot struct {
	startTimes   StartTimes
	measurements MeasurementMap
}

// NewThroughputVSLatencyPlot returns a new throughput plotter.
func NewThroughputVSLatencyPlot() ThroughputVSLatencyPlot {
	return ThroughputVSLatencyPlot{
		startTimes:   NewStartTimes(),
		measurements: NewMeasurementMap(),
	}
}

// Add adds a measurement to the plotter.
func (p *ThroughputVSLatencyPlot) Add(measurement interface{}) {
	p.startTimes.Add(measurement)

	m, ok := measurement.(Measurement)
	if !ok {
		return
	}

	id := m.GetEvent().GetID()

	switch measurement.(type) {
	case *types.LatencyMeasurement:
		if !m.GetEvent().GetClient() {
			// ignore replica latency
			return
		}
	case *types.ThroughputMeasurement:
		if m.GetEvent().GetClient() {
			// ignore client throughput
			return
		}
	}

	p.measurements.Add(id, m)
}

// PlotAverage plots the average throughput of all replicas at specified time intervals.
func (p *ThroughputVSLatencyPlot) PlotAverage(filename string, measurementInterval time.Duration) (err error) {
	plt, err := plot.New()
	if err != nil {
		return fmt.Errorf("failed to create plot: %w", err)
	}

	grid := plotter.NewGrid()
	grid.Horizontal.Color = color.Gray{Y: 200}
	grid.Horizontal.Dashes = plotutil.Dashes(2)
	grid.Vertical.Color = color.Gray{Y: 200}
	grid.Vertical.Dashes = plotutil.Dashes(2)
	plt.Add(grid)

	plt.X.Label.Text = "Throughput (commands/second)"
	plt.X.Tick.Marker = hplot.Ticks{N: 10}
	plt.Y.Label.Text = "Latency (milliseconds)"
	plt.Y.Tick.Marker = hplot.Ticks{N: 10}

	// TODO: error bars
	if err := plotutil.AddScatters(plt, avgThroughputVSAvgLatency(p, measurementInterval)); err != nil {
		return fmt.Errorf("failed to add line plot: %w", err)
	}

	if err := plt.Save(6*vg.Inch, 6*vg.Inch, filename); err != nil {
		return fmt.Errorf("failed to save plot: %w", err)
	}

	return nil
}

func avgThroughputVSAvgLatency(p *ThroughputVSLatencyPlot, interval time.Duration) plotter.XYer {
	groups := GroupByTimeInterval(&p.startTimes, p.measurements, interval)
	points := make(xyer, 0, len(groups))
	for _, group := range groups {
		var (
			latencySum    float64
			latencyNum    uint64
			throughputSum float64
			throughputNum uint64
		)
		for _, measurement := range group.Measurements {
			switch m := measurement.(type) {
			case *types.LatencyMeasurement:
				latencySum += m.GetLatency() * float64(m.GetCount())
				latencyNum += m.GetCount()
			case *types.ThroughputMeasurement:
				throughputSum += float64(m.GetCommands()) / m.GetDuration().AsDuration().Seconds()
				throughputNum++
			}
		}
		if throughputNum > 0 && latencyNum > 0 {
			points = append(points, point{
				x: throughputSum / float64(throughputNum),
				y: latencySum / float64(latencyNum),
			})
		}
	}
	return points
}
