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

type ThroughputPlot struct {
	startTimes   StartTimes
	measurements map[uint32][]*types.ThroughputMeasurement
}

func NewThroughputPlot() ThroughputPlot {
	return ThroughputPlot{
		startTimes:   NewStartTimes(),
		measurements: make(map[uint32][]*types.ThroughputMeasurement),
	}
}

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
	p.measurements[id] = append(p.measurements[id], throughput)
}

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
	if err := plotutil.AddLinePoints(plt, avgThroughputIter(p, measurementInterval)); err != nil {
		return fmt.Errorf("failed to add line plot: %w", err)
	}

	if err := plt.Save(6*vg.Inch, 6*vg.Inch, filename); err != nil {
		return fmt.Errorf("failed to save plot: %w", err)
	}

	return nil
}

type throughputDataPoint struct {
	time       float64
	throughput float64
}

type avgThroughput struct {
	dataPoints []throughputDataPoint
}

func avgThroughputIter(p *ThroughputPlot, interval time.Duration) *avgThroughput {
	indices := make([]int, len(p.measurements))
	throughput := avgThroughput{}

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
					sum += float64(m.GetCommits()) / m.GetDuration().AsDuration().Seconds()
					num++
					indices[i]++
				} else {
					break
				}
			}
			i++
		}
		if num > 0 {
			throughput.dataPoints = append(throughput.dataPoints, throughputDataPoint{
				time:       currentTime.Seconds(),
				throughput: sum / float64(num),
			})
		}
		if measurementsRemaining == 0 {
			break
		}
		currentTime += interval
	}

	return &throughput
}

// Len returns the number of x, y pairs.
func (t *avgThroughput) Len() int {
	return len(t.dataPoints)
}

// XY returns an x, y pair.
func (t *avgThroughput) XY(i int) (x float64, y float64) {
	dp := t.dataPoints[i]
	return dp.time, dp.throughput
}
