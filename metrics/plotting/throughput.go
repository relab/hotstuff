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

// ThroughputPlot is a plotter that plots throughput vs time.
type ThroughputPlot struct {
	startTimes   StartTimes
	measurements MeasurementMap
}

// NewThroughputPlot returns a new throughput plotter.
func NewThroughputPlot() ThroughputPlot {
	return ThroughputPlot{
		startTimes:   NewStartTimes(),
		measurements: NewMeasurementMap(),
	}
}

// Add adds a measurement to the plotter.
func (p *ThroughputPlot) Add(measurement interface{}) {
	p.startTimes.Add(measurement)

	throughput, ok := measurement.(*types.ThroughputMeasurement)
	if !ok {
		return
	}

	if throughput.GetEvent().GetClient() {
		// ignoring client events
		return
	}

	id := throughput.GetEvent().GetID()
	p.measurements.Add(id, throughput)
}

// PlotAverage plots the average throughput of all replicas at specified time intervals.
func (p *ThroughputPlot) PlotAverage(filename string, measurementInterval time.Duration) (err error) {
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

	plt.X.Label.Text = "Time (seconds)"
	plt.X.Tick.Marker = hplot.Ticks{N: 10}
	plt.Y.Label.Text = "Throughput (commands/second)"
	plt.Y.Tick.Marker = hplot.Ticks{N: 10}

	// TODO: error bars
	if err := plotutil.AddLinePoints(plt, avgThroughput(p, measurementInterval)); err != nil {
		return fmt.Errorf("failed to add line plot: %w", err)
	}

	if err := plt.Save(6*vg.Inch, 6*vg.Inch, filename); err != nil {
		return fmt.Errorf("failed to save plot: %w", err)
	}

	return nil
}

func avgThroughput(p *ThroughputPlot, interval time.Duration) plotter.XYer {
	intervals := GroupByTimeInterval(&p.startTimes, p.measurements, interval)
	return TimeAndAverage(intervals, func(m Measurement) (float64, uint64) {
		tp := m.(*types.ThroughputMeasurement)
		return float64(tp.GetCommands()) / tp.GetDuration().AsDuration().Seconds(), 1
	})
}
