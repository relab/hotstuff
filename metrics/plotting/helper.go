package plotting

import (
	"encoding/csv"
	"fmt"
	"image/color"
	"os"
	"time"

	"github.com/relab/hotstuff/metrics/types"
	"go-hep.org/x/hep/hplot"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

// GonumPlot sets up a gonum/plot and calls f to add data.
func GonumPlot(filename, xlabel, ylabel string, f func(plt *plot.Plot) error) error {
	plt := plot.New()

	grid := plotter.NewGrid()
	grid.Horizontal.Color = color.Gray{Y: 200}
	grid.Horizontal.Dashes = plotutil.Dashes(2)
	grid.Vertical.Color = color.Gray{Y: 200}
	grid.Vertical.Dashes = plotutil.Dashes(2)
	plt.Add(grid)

	plt.X.Label.Text = xlabel
	plt.X.Tick.Marker = hplot.Ticks{N: 10}
	plt.Y.Label.Text = ylabel
	plt.Y.Tick.Marker = hplot.Ticks{N: 10}

	if err := f(plt); err != nil {
		return err
	}

	if err := plt.Save(6*vg.Inch, 6*vg.Inch, filename); err != nil {
		return fmt.Errorf("failed to save plot: %w", err)
	}

	return nil
}

// MeasurementMap is a map that stores lists Measurement objects associated
// with the ID of the client/replica where they where taken.
type MeasurementMap struct {
	m map[uint32][]Measurement
}

// NewMeasurementMap constructs a new MeasurementMap
func NewMeasurementMap() MeasurementMap {
	return MeasurementMap{m: make(map[uint32][]Measurement)}
}

// Add adds a measurement to the map.
func (m *MeasurementMap) Add(id uint32, measurement Measurement) {
	m.m[id] = append(m.m[id], measurement)
}

// Get returns the list of measurements associated with the specified client/replica id.
func (m *MeasurementMap) Get(id uint32) (measurements []Measurement, ok bool) {
	measurements, ok = m.m[id]
	return
}

// NumIDs returns the number of client/replica IDs that are registered in the map.
func (m *MeasurementMap) NumIDs() int {
	return len(m.m)
}

// Measurement is an object with a types.Event getter.
type Measurement interface {
	GetEvent() *types.Event
}

// MeasurementGroup is a collection of measurements that were taken within a time interval.
type MeasurementGroup struct {
	Time         time.Duration // The beginning of the time interval
	Measurements []Measurement
}

// GroupByTimeInterval merges all measurements from all client/replica ids into groups based on the time interval that
// the measurement was taken in. The StartTimes object is used to calculate which time interval a measurement falls in.
func GroupByTimeInterval(startTimes *StartTimes, m MeasurementMap, interval time.Duration) []MeasurementGroup {
	var (
		indices     = make([]int, m.NumIDs()) // the index within each client/replica measurement list
		groups      []MeasurementGroup        // the groups we are creating
		currentTime time.Duration             // the start of the current time interval
	)
	for {
		var (
			i         int                                   // index into indices
			remaining int                                   // number of measurements remaining to be processed
			group     = MeasurementGroup{Time: currentTime} // the group of measurements within the current time interval
		)
		for _, measurements := range m.m {
			remaining += len(measurements) - indices[i]
			for indices[i] < len(measurements) {
				m := measurements[indices[i]]
				// check if this measurement falls within the current time interval
				t, ok := startTimes.ClientOffset(m.GetEvent().GetID(), m.GetEvent().GetTimestamp().AsTime())
				if ok && t < currentTime+interval {
					// add it to the group and move to the next measurement
					group.Measurements = append(group.Measurements, m)
					indices[i]++
				} else {
					// the measurement will be processed later
					break
				}
			}
			i++
		}
		if len(group.Measurements) > 0 {
			groups = append(groups, group)
		}
		if remaining <= 0 {
			break
		}
		currentTime += interval
	}
	return groups
}

// TimeAndAverage returns a struct that yields (x, y) points where x is the time,
// and y is the average value of each group. The getValue function must return the
// value and sample count for the given measurement.
func TimeAndAverage(groups []MeasurementGroup, getValue func(Measurement) (float64, uint64)) plotter.XYer {
	points := make(xyer, 0, len(groups))
	for _, group := range groups {
		var (
			sum float64
			num uint64
		)
		for _, measurement := range group.Measurements {
			v, n := getValue(measurement)
			sum += v * float64(n)
			num += n
		}
		if num > 0 {
			points = append(points, point{
				x: group.Time.Seconds(),
				y: sum / float64(num),
			})
		}
	}
	return points
}

type point struct {
	x float64
	y float64
}

type xyer []point

// Len returns the number of x, y pairs.
func (xy xyer) Len() int {
	return len(xy)
}

// XY returns an x, y pair.
func (xy xyer) XY(i int) (x float64, y float64) {
	p := xy[i]
	return p.x, p.y
}

// CSVPlot writes to a CSV file.
func CSVPlot(filename string, headers []string, plot func() plotter.XYer) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	wr := csv.NewWriter(f)
	err = wr.Write(headers)
	if err != nil {
		return err
	}
	xyer := plot()
	for i := 0; i < xyer.Len(); i++ {
		x, y := xyer.XY(i)
		err = wr.Write([]string{fmt.Sprint(x), fmt.Sprint(y)})
		if err != nil {
			return err
		}
	}
	wr.Flush()
	return f.Close()
}
