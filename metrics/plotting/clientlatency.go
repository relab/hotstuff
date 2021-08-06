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
	measurements map[uint32][]*types.LatencyMeasurement
}

// NewClientLatencyPlot returns a new client latency plotter.
func NewClientLatencyPlot() ClientLatencyPlot {
	return ClientLatencyPlot{
		startTimes:   NewStartTimes(),
		measurements: make(map[uint32][]*types.LatencyMeasurement),
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
	p.measurements[id] = append(p.measurements[id], latency)
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
	if err := plotutil.AddLinePoints(plt, avgLatencyIter(p, measurementInterval)); err != nil {
		return fmt.Errorf("failed to add line plot: %w", err)
	}

	if err := plt.Save(6*vg.Inch, 6*vg.Inch, filename); err != nil {
		return fmt.Errorf("failed to save plot: %w", err)
	}

	return nil
}

type dataPoint struct {
	time    float64
	latency float64
}

type avgLatency struct {
	dataPoints []dataPoint
}

func avgLatencyIter(p *ClientLatencyPlot, interval time.Duration) *avgLatency {
	indices := make([]int, len(p.measurements))
	latencies := avgLatency{}

	var currentTime time.Duration
	for {
		var (
			sum float64
			num int
			i   int
		)

		// sum all measurements that fall within the current time interval
		measurementsRemaining := 0
		for _, measurements := range p.measurements {
			measurementsRemaining += len(measurements) - indices[i]
			for indices[i] < len(measurements) {
				m := measurements[indices[i]]
				t, ok := p.startTimes.ClientOffset(m.GetEvent().GetID(), m.GetEvent().GetTimestamp().AsTime())
				if ok && t < currentTime+interval {
					sum += m.GetLatency() * float64(m.GetCount())
					num += int(m.GetCount())
					indices[i]++
				} else {
					break
				}
			}
			i++
		}
		if num > 0 {
			latencies.dataPoints = append(latencies.dataPoints, dataPoint{
				time:    currentTime.Seconds(),
				latency: sum / float64(num),
			})
		}
		if measurementsRemaining == 0 {
			break
		}
		currentTime += interval
	}

	return &latencies
}

// Len returns the number of x, y pairs.
func (l *avgLatency) Len() int {
	return len(l.dataPoints)
}

// XY returns an x, y pair.
func (l *avgLatency) XY(i int) (x float64, y float64) {
	dp := l.dataPoints[i]
	return dp.time, dp.latency
}
