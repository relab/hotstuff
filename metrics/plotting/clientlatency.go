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

// ClientLatencyPlot plots client latency measurements.
type ClientLatencyPlot struct {
	startTimes   StartTimes
	measurements MeasurementMap
}

// NewClientLatencyPlot returns a new client latency plotter.
func NewClientLatencyPlot() ClientLatencyPlot {
	return ClientLatencyPlot{
		startTimes:   NewStartTimes(),
		measurements: NewMeasurementMap(),
	}
}

// Add adds a measurement to the plot.
func (p *ClientLatencyPlot) Add(measurement interface{}) {
	p.startTimes.Add(measurement)

	latency, ok := measurement.(*types.LatencyMeasurement)
	if !ok {
		return
	}

	// only care about client's latency
	if !latency.GetEvent().GetClient() {
		return
	}
	id := latency.GetEvent().GetID()
	p.measurements.Add(id, latency)
}

// PlotAverage plots the average latency of all clients within each measurement interval.
func (p *ClientLatencyPlot) PlotAverage(filename string, measurementInterval time.Duration) (err error) {
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
	plt.Y.Label.Text = "Latency (ms)"
	plt.Y.Tick.Marker = hplot.Ticks{N: 10}

	// TODO: error bars
	if err := plotutil.AddLinePoints(plt, avgLatency(p, measurementInterval)); err != nil {
		return fmt.Errorf("failed to add line plot: %w", err)
	}

	if err := plt.Save(6*vg.Inch, 6*vg.Inch, filename); err != nil {
		return fmt.Errorf("failed to save plot: %w", err)
	}

	return nil
}

func avgLatency(p *ClientLatencyPlot, interval time.Duration) plotter.XYer {
	intervals := GroupByTimeInterval(&p.startTimes, p.measurements, interval)
	return TimeAndAverage(intervals, func(m Measurement) (float64, uint64) {
		latency := m.(*types.LatencyMeasurement)
		return latency.GetLatency(), latency.GetCount()
	})
}
