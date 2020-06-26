package main

import (
	"bufio"
	"flag"
	"fmt"
	"image/color"
	"io/ioutil"
	"math"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"go-hep.org/x/hep/hplot"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

type dataPoint struct {
	deltaTime time.Duration
	latency   time.Duration
}

type measurement struct {
	rate               int
	latency            float64
	latencyVariance    float64
	throughput         float64
	throughputVariance float64
	numCommands        int
	numThrMeasurements int
}

type benchmark struct {
	measurements []measurement
	name         string
	batchSize    int
	payloadSize  int
}

func (b benchmark) String() string {
	var ret strings.Builder
	ret.WriteString("------------------------------------------------------------\n")
	ret.WriteString(fmt.Sprintf("%s: batch size b%d, payload size p%d\n", b.name, b.batchSize, b.payloadSize))
	ret.WriteString("Throughput mean, Throughput Stdev, Latency mean, Latency Stdev\n")
	for _, m := range b.measurements {
		ret.WriteString(fmt.Sprintf("%.2f, %.2f, %.2f, %.2f\n",
			m.throughput, math.Sqrt(m.throughputVariance), m.latency, math.Sqrt(m.latencyVariance)))
	}
	return ret.String()
}

func (b *benchmark) Len() int {
	return len(b.measurements)
}

func (b *benchmark) XY(i int) (x, y float64) {
	m := b.measurements[i]
	return m.throughput, m.latency
}

func (b *benchmark) XError(i int) (lower, upper float64) {
	m := b.measurements[i]
	lower = -1.96 * math.Sqrt(m.throughputVariance) / math.Sqrt(float64(m.numThrMeasurements))
	if math.IsNaN(lower) {
		lower = 0
	}
	upper = 1.96 * math.Sqrt(m.throughputVariance) / math.Sqrt(float64(m.numThrMeasurements))
	if math.IsNaN(upper) {
		upper = 0
	}
	return
}

func (b *benchmark) YError(i int) (lower, upper float64) {
	m := b.measurements[i]
	lower = -1.96 * math.Sqrt(m.latencyVariance) / math.Sqrt(float64(m.numCommands))
	if math.IsNaN(lower) {
		lower = 0
	}
	upper = 1.96 * math.Sqrt(m.latencyVariance) / math.Sqrt(float64(m.numCommands))
	if math.IsNaN(upper) {
		lower = 0
	}
	return
}

func main() {
	plotFile := flag.String("plot", "", "Destination to save plot in")
	conf95 := flag.Bool("conf95", false, "Draw a 95% confidence interval to each measurement")
	flag.Parse()

	if len(flag.Args()) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [paths to benchmark files]\n", os.Args[0])
		os.Exit(1)
	}

	var plots []interface{}
	var errorBars []interface{}
	benchmarks := make(chan *benchmark, 1)
	errors := make(chan error, 1)
	n := 0

	for _, benchFolder := range flag.Args() {
		dir, err := ioutil.ReadDir(benchFolder)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open benchmark directory: %v\n", err)
			os.Exit(1)
		}

		for _, f := range dir {
			if f.IsDir() {
				go func(dir string) {
					b, err := processBenchmark(dir)
					benchmarks <- b
					errors <- err
				}(path.Join(benchFolder, f.Name()))
				n++
			}
		}
	}

	for i := 0; i < n; i++ {
		err := <-errors
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read benchmark: %v\n", err)
			os.Exit(1)
		}

		b := <-benchmarks
		label := fmt.Sprintf("%s-b%d-p%d", b.name, b.batchSize, b.payloadSize)

		// insert sorted for deterministic output
		index := len(plots)
		for j, v := range plots {
			if s, ok := v.(string); ok {
				if label > s {
					index = j
				}
			}
		}
		// grow size by 2
		plots = append(plots, struct{}{}, struct{}{})
		copy(plots[index+2:], plots[index:])
		plots[index] = label
		plots[index+1] = b
	}

	for _, p := range plots {
		if _, ok := p.(string); !ok {
			errorBars = append(errorBars, p)
		}
	}

	if *plotFile != "" {
		p, err := plot.New()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create plot: %v\n", err)
			os.Exit(1)
		}

		grid := plotter.NewGrid()
		grid.Horizontal.Color = color.Gray{Y: 200}
		grid.Horizontal.Dashes = plotutil.Dashes(2)
		grid.Vertical.Color = color.Gray{Y: 200}
		grid.Vertical.Dashes = plotutil.Dashes(2)
		p.Add(grid)

		err = plotutil.AddLinePoints(p, plots...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to add plots: %v\n", err)
			os.Exit(1)
		}

		if *conf95 {
			err = plotutil.AddErrorBars(p, errorBars...)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to add error bars: %v\n", err)
				os.Exit(1)
			}
		}

		p.Legend.Left = true
		p.Legend.Top = true
		p.X.Label.Text = "Throughput (Kops/sec)"
		p.X.Tick.Marker = hplot.Ticks{N: 10}
		p.Y.Label.Text = "Latency (ms)"
		p.Y.Tick.Marker = hplot.Ticks{N: 10}

		if err := p.Save(6*vg.Inch, 6*vg.Inch, *plotFile); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save plot: %v\n", err)
			os.Exit(1)
		}
	} else {
		// just print the benchmarks to stdout
		for _, p := range plots {
			if b, ok := p.(*benchmark); ok {
				fmt.Println(b)
			}
		}
	}
}

func processBenchmark(dirPath string) (*benchmark, error) {
	measurements := make(map[int][]measurement)
	dir, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	b := &benchmark{}
	if strings.HasPrefix(path.Base(dirPath), "lhs-") {
		b.name = "libhotstuff"
	} else {
		b.name = "relab/hotstuff"
	}
	fmt.Sscanf(strings.TrimPrefix(path.Base(dirPath), "lhs-"), "b%d-p%d", &b.batchSize, &b.payloadSize)
	for _, f := range dir {
		if f.IsDir() {
			if err := processRun(path.Join(dirPath, f.Name()), measurements, b.name); err != nil {
				return nil, err
			}
		}
	}
	b.measurements = make([]measurement, 0, len(measurements))
	for _, ms := range measurements {
		m := combineMeasurements(false, ms...)
		i := sort.Search(len(b.measurements), func(i int) bool {
			return m.rate < b.measurements[i].rate
		})
		b.measurements = append(b.measurements, measurement{})
		copy(b.measurements[i+1:], b.measurements[i:])
		b.measurements[i] = m
	}
	return b, nil
}

func processRun(dirPath string, measurements map[int][]measurement, benchType string) error {
	dir, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return err
	}

	ms := make(chan measurement, len(dir))
	errs := make(chan error, len(dir))
	n := 0

	for _, f := range dir {
		if f.IsDir() {
			go func(dirPath string) {
				var (
					m   measurement
					err error
				)

				if benchType == "libhotstuff" {
					m, err = readInMeasurement(dirPath, false)
				} else {
					m, err = readInMeasurement(dirPath, true)
				}

				fmt.Sscanf(path.Base(dirPath), "t%d", &m.rate)

				errs <- err
				ms <- m

			}(path.Join(dirPath, f.Name()))
			n++
		}
	}

	for i := 0; i < n; i++ {
		err := <-errs
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: error while processing measurement (ignoring): %v\n", err)
			<-ms
			continue
		}
		m := <-ms
		s := measurements[m.rate]
		s = append(s, m)
		measurements[m.rate] = s
	}

	return nil
}

func readInMeasurement(dirPath string, nanoseconds bool) (measurement, error) {
	dir, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return measurement{}, err
	}

	var data [][]dataPoint
	for _, file := range dir {
		if file.IsDir() {
			dirPath := path.Join(dirPath, file.Name())
			dir, err := ioutil.ReadDir(dirPath)
			if err != nil {
				return measurement{}, err
			}
			for _, file := range dir {
				if file.IsDir() {
					continue
				}
				d, err := readInData(path.Join(dirPath, file.Name()), nanoseconds)
				if err != nil {
					return measurement{}, err
				}
				data = append(data, d)
			}
			continue
		}

		d, err := readInData(path.Join(dirPath, file.Name()), nanoseconds)
		if err != nil {
			return measurement{}, err
		}
		data = append(data, d)
	}

	var totalLatency time.Duration
	var throughput float64
	numCommands := 0
	for _, d := range data {
		if len(d) == 0 {
			continue
		}
		var totalTime time.Duration
		numCommands += len(d)
		for _, p := range d {
			totalLatency += p.latency / time.Millisecond
			totalTime += p.deltaTime
		}
		throughput += (float64(len(d)) / 1000) / totalTime.Seconds()
	}
	latencyMean := (float64(totalLatency) / float64(numCommands))
	latencyVariance := 0.0
	for _, d := range data {
		for _, p := range d {
			latencyVariance += math.Pow(float64(p.latency/time.Millisecond)-latencyMean, 2)
		}
	}
	latencyVariance /= float64(numCommands - 1)

	return measurement{
		latency:            latencyMean,
		latencyVariance:    latencyVariance,
		throughput:         throughput,
		numCommands:        len(data),
		numThrMeasurements: 1,
	}, nil
}

func readInData(filePath string, nanoseconds bool) ([]dataPoint, error) {
	var numberFormat string
	if nanoseconds {
		numberFormat = "%sns"
	} else {
		numberFormat = "%sus"
	}
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	var data []dataPoint
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		l := scanner.Text()
		matches := strings.Split(l, " ")
		if len(matches) < 2 {
			continue
		}
		t, err := time.ParseDuration(fmt.Sprintf("%sns", matches[0]))
		if err != nil {
			return nil, fmt.Errorf("Failed to read data: %w", err)
		}
		lat, err := time.ParseDuration(fmt.Sprintf(numberFormat, matches[1]))
		if err != nil {
			return nil, fmt.Errorf("Failed to read data: %w", err)
		}
		data = append(data, dataPoint{deltaTime: t, latency: lat})
	}
	return data, nil
}

func combineMeasurements(sumThroughput bool, measurements ...measurement) measurement {
	if len(measurements) == 0 {
		panic("combineMeasurements called with no measurements!")
	}

	if len(measurements) == 1 {
		return measurements[0]
	}

	rate := measurements[0].rate

	// first need to calculate mean
	latencyMean := 0.0
	throughput := 0.0
	numCommands := 0
	numThrMeasurements := 0
	for _, m := range measurements {
		if m.rate != rate {
			panic("Attempt to combine measurements with different rate.")
		}
		latencyMean += float64(m.numCommands) * m.latency
		throughput += float64(m.numThrMeasurements) * m.throughput
		numCommands += m.numCommands
		numThrMeasurements += m.numThrMeasurements
	}
	latencyMean /= float64(numCommands)
	throughputMean := throughput / float64(numThrMeasurements)

	// now calculate variance
	latencyVariance := 0.0
	throughputVariance := 0.0
	for _, m := range measurements {
		latencyVariance += float64(m.numCommands-1) * (m.latencyVariance + m.latency - latencyMean)
		throughputVariance += math.Pow(m.throughput-throughputMean, 2)
	}
	latencyVariance /= float64(numCommands - len(measurements))
	throughputVariance /= float64(len(measurements) - 1)

	return measurement{
		rate:               rate,
		latency:            latencyMean,
		latencyVariance:    latencyVariance,
		throughput:         throughputMean,
		throughputVariance: throughputVariance,
		numCommands:        numCommands,
		numThrMeasurements: numThrMeasurements,
	}
}
