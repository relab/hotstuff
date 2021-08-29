package plotting

import (
	"fmt"
	"path"
	"time"

	"github.com/relab/hotstuff/metrics/types"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
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
	const (
		xlabel = "Time (seconds)"
		ylabel = "Throughput (commands/second)"
	)
	if path.Ext(filename) == ".csv" {
		return CSVPlot(filename, []string{xlabel, ylabel}, func() plotter.XYer {
			return avgThroughput(p, measurementInterval)
		})
	}
	return GonumPlot(filename, xlabel, ylabel, func(plt *plot.Plot) error {
		if err := plotutil.AddLinePoints(plt, avgThroughput(p, measurementInterval)); err != nil {
			return fmt.Errorf("failed to add line plot: %w", err)
		}
		return nil
	})
}

func avgThroughput(p *ThroughputPlot, interval time.Duration) plotter.XYer {
	intervals := GroupByTimeInterval(&p.startTimes, p.measurements, interval)
	return TimeAndAverage(intervals, func(m Measurement) (float64, uint64) {
		tp := m.(*types.ThroughputMeasurement)
		return float64(tp.GetCommands()) / tp.GetDuration().AsDuration().Seconds(), 1
	})
}
