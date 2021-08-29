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
	const (
		xlabel = "Time (seconds)"
		ylabel = "Latency (ms)"
	)
	if path.Ext(filename) == ".csv" {
		return CSVPlot(filename, []string{xlabel, ylabel}, func() plotter.XYer {
			return avgLatency(p, measurementInterval)
		})
	}
	return GonumPlot(filename, xlabel, ylabel, func(plt *plot.Plot) error {
		// TODO: error bars
		if err := plotutil.AddLinePoints(plt, avgLatency(p, measurementInterval)); err != nil {
			return fmt.Errorf("failed to add line plot: %w", err)
		}
		return nil
	})
}

func avgLatency(p *ClientLatencyPlot, interval time.Duration) plotter.XYer {
	intervals := GroupByTimeInterval(&p.startTimes, p.measurements, interval)
	return TimeAndAverage(intervals, func(m Measurement) (float64, uint64) {
		latency := m.(*types.LatencyMeasurement)
		return latency.GetLatency(), latency.GetCount()
	})
}
