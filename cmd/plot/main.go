package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"go-hep.org/x/hep/hplot"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

type rawThroughput struct {
	deltaTime   float64
	numCommands float64
}

type measurement struct {
	rate       int
	latency    float64
	throughput float64
}

type benchmark struct {
	measurements []measurement
	name         string
	batchSize    int
	payloadSize  int
}

func (b benchmark) String() string {
	var ret strings.Builder
	ret.WriteString(fmt.Sprintf("%s-b%d-p%d: ", b.name, b.batchSize, b.payloadSize))
	for _, m := range b.measurements {
		ret.WriteString(fmt.Sprintf("(%f, %f), ", m.throughput, m.latency))
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

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s [paths to benchmark files] [output file]\n", os.Args[0])
		os.Exit(1)
	}

	p, err := plot.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create plot: %v\n", err)
		os.Exit(1)
	}

	var plots []interface{}
	benchmarks := make(chan *benchmark, 1)
	errors := make(chan error, 1)
	n := 0

	for _, benchFolder := range os.Args[1 : len(os.Args)-1] {
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

	err = plotutil.AddLinePoints(p, plots...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to add plots: %v\n", err)
		os.Exit(1)
	}

	/* p.Legend.Left = true */
	p.Legend.Top = true
	p.X.Label.Text = "Throughput Kops/sec"
	p.X.Tick.Marker = hplot.Ticks{N: 8}
	p.Y.Label.Text = "Latency ms"
	p.Y.Tick.Marker = hplot.Ticks{N: 5}
	p.Add(plotter.NewGrid())

	if err := p.Save(4*vg.Inch, 4*vg.Inch, os.Args[len(os.Args)-1]); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save plot: %v\n", err)
		os.Exit(1)
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
		latencyTotal, throughputTotal := 0.0, 0.0
		rate := ms[0].rate
		for _, m := range ms {
			if m.rate != rate {
				panic("Rate mismatch while combining measurements!")
			}
			latencyTotal += m.latency
			throughputTotal += m.throughput
		}
		m := measurement{
			rate:       rate,
			latency:    latencyTotal / float64(len(ms)),
			throughput: throughputTotal / float64(len(ms)),
		}
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
					m, err = readInLibHotStuffMeasurement(dirPath)
				} else {
					m, err = processMeasurement(dirPath)
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
			return err
		}
		m := <-ms
		s, _ := measurements[m.rate]
		s = append(s, m)
		measurements[m.rate] = s
	}

	return nil
}

func processMeasurement(dirPath string) (measurement, error) {
	dir, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return measurement{}, err
	}
	var latencies []float64
	var throughput []rawThroughput
	for _, f := range dir {
		if f.IsDir() {
			continue
		}
		latenciesReg := regexp.MustCompile(`client-\d+\.out`)
		throughputReg := regexp.MustCompile(`hotstuff-\d+\.out`)
		switch {
		case latenciesReg.MatchString(f.Name()):
			err := readInLatencies(path.Join(dirPath, f.Name()), &latencies)
			if err != nil {
				return measurement{}, err
			}
		case throughputReg.MatchString(f.Name()):
			err := readInThroughput(path.Join(dirPath, f.Name()), &throughput)
			if err != nil {
				return measurement{}, err
			}
		}
	}

	latencySum := 0.0
	for _, l := range latencies {
		latencySum += l
	}
	latencyAvg := (latencySum / float64(len(latencies))) / float64(time.Millisecond)

	totalTime, totalCommands := 0.0, 0.0
	for _, t := range throughput {
		totalTime += t.deltaTime
		totalCommands += t.numCommands
	}

	throughputAvg := (totalCommands / 1000) / (totalTime / float64(time.Second))

	if math.IsNaN(throughputAvg) || math.IsNaN(latencyAvg) {
		return measurement{}, fmt.Errorf("NaN measurement in '%s'", dirPath)
	}

	return measurement{latency: latencyAvg, throughput: throughputAvg}, nil
}

func readInLatencies(file string, latencies *[]float64) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		l := scanner.Text()
		var m float64
		n, err := fmt.Sscanf(l, "%f", &m)
		if n != 1 {
			return fmt.Errorf("Failed to read latency measurement: %w", err)
		}
		*latencies = append(*latencies, float64(m))
	}
	return nil
}

func readInThroughput(file string, throughput *[]rawThroughput) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		l := scanner.Text()
		var d, t float64
		n, err := fmt.Sscanf(l, "%f,%f", &d, &t)
		if n != 2 {
			return fmt.Errorf("Failed to read throughput measurement: %w", err)
		}
		if t > 0 {
			*throughput = append(*throughput, rawThroughput{deltaTime: d, numCommands: t})
		}
	}
	return nil
}

func readInLibHotStuffMeasurement(dirPath string) (measurement, error) {
	re := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}.\d{6}) \[hotstuff info\] (\d+?.\d+?)$`)

	dir, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return measurement{}, err
	}

	var measurements []measurement
	for _, file := range dir {
		if file.IsDir() {
			m, err := readInLibHotStuffMeasurement(path.Join(dirPath, file.Name()))
			if err != nil {
				return measurement{}, err
			}
			measurements = append(measurements, m)
			continue
		}

		f, err := os.Open(path.Join(dirPath, file.Name()))
		if err != nil {
			return measurement{}, fmt.Errorf("Failed to read libhotstuff measurement: %w", err)
		}

		var totalLatency time.Duration
		var prevTime *time.Time
		var totalTime time.Duration
		numCommands := 0
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			l := scanner.Text()
			matches := re.FindStringSubmatch(l)
			if len(matches) < 3 {
				continue
			}
			t, err := time.Parse("2006-01-02 15:04:05.999999", matches[1])
			if err != nil {
				return measurement{}, fmt.Errorf("Failed to read libhotstuff measurement: %w", err)
			}
			lat := matches[2]
			d, err := time.ParseDuration(fmt.Sprintf("%ss", lat))
			if err != nil {
				return measurement{}, fmt.Errorf("Failed to read libhotstuff measurement: %w", err)
			}
			numCommands++
			totalLatency += d
			if prevTime != nil {
				totalTime += t.Sub(*prevTime)
			}
			prevTime = &t
		}
		latency := (float64(totalLatency) / float64(numCommands)) / float64(time.Millisecond)
		throughput := (float64(numCommands) / 1000) / totalTime.Seconds()
		measurements = append(measurements, measurement{latency: latency, throughput: throughput})
	}

	if len(measurements) == 0 {
		return measurement{}, fmt.Errorf("No measurements")
	}

	sumLatencies := 0.0
	sumThroughput := 0.0
	for _, m := range measurements {
		if math.IsNaN(m.throughput) || math.IsNaN(m.latency) {
			fmt.Fprintf(os.Stderr, "WARNING: Ignoring NaN measurement in '%s'\n", dirPath)
			continue
		}
		sumLatencies += m.latency
		sumThroughput += m.throughput
	}

	return measurement{
		latency:    sumLatencies / float64(len(measurements)),
		throughput: sumThroughput,
	}, nil
}
