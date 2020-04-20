package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"sort"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

type rawThroughput struct {
	deltaTime   float64
	numCommands float64
}

type measurement struct {
	latency    float64
	throughput float64
}

type benchmark struct {
	measurements []measurement
	batchSize    int
	payloadSize  int
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
		fmt.Fprintf(os.Stderr, "Usage: %s [path to benchmark files] [output file]\n", os.Args[0])
		os.Exit(1)
	}

	var plots []interface{}

	benchFolder := os.Args[1]
	dir, err := ioutil.ReadDir(benchFolder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open benchmark directory: %v\n", err)
		os.Exit(1)
	}
	for _, f := range dir {
		if f.IsDir() {
			b, err := processBenchmark(path.Join(benchFolder, f.Name()))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read benchmark: %v\n", err)
				os.Exit(1)
			}
			plots = append(plots, fmt.Sprintf("hs-b%d-p%d", b.batchSize, b.payloadSize), b)
		}
	}

	p, err := plot.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create plot: %v\n", err)
		os.Exit(1)
	}

	err = plotutil.AddLinePoints(p, plots...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to add plots: %v\n", err)
		os.Exit(1)
	}

	p.Legend.Left = true
	p.Legend.Top = true
	p.X.Label.Text = "Throughput Kops/sec"
	p.Y.Label.Text = "Latency ms"

	if err := p.Save(4*vg.Inch, 4*vg.Inch, os.Args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save plot: %v\n", err)
		os.Exit(1)
	}
}

func processBenchmark(dirPath string) (*benchmark, error) {
	measurements := make(map[string][]measurement)
	dir, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	b := &benchmark{}
	fmt.Sscanf(path.Base(dirPath), "b%d-p%d", &b.batchSize, &b.payloadSize)
	for _, f := range dir {
		if f.IsDir() {
			if err := processRun(path.Join(dirPath, f.Name()), measurements); err != nil {
				return nil, err
			}
		}
	}
	b.measurements = make([]measurement, 0, len(measurements))
	for _, ms := range measurements {
		latencyTotal, throughputTotal := 0.0, 0.0
		for _, m := range ms {
			latencyTotal += m.latency
			throughputTotal += m.throughput
		}
		m := measurement{
			latency:    latencyTotal / float64(len(ms)),
			throughput: throughputTotal / float64(len(ms)),
		}
		i := sort.Search(len(b.measurements), func(i int) bool {
			return b.measurements[i].throughput >= m.throughput
		})
		b.measurements = append(b.measurements, measurement{})
		copy(b.measurements[i+1:], b.measurements[i:])
		b.measurements[i] = m
	}
	return b, nil
}

func processRun(dirPath string, measurements map[string][]measurement) error {
	dir, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return err
	}
	for _, f := range dir {
		if f.IsDir() {
			m, err := processMeasurement(path.Join(dirPath, f.Name()))
			if err != nil {
				return err
			}
			s, _ := measurements[f.Name()]
			s = append(s, m)
			measurements[f.Name()] = s
		}
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
			fmt.Fprintf(os.Stderr, "Failed to read latency measurement: %v\n", err)
			continue
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
			fmt.Fprintf(os.Stderr, "Failed to read throughput measurement: %v\n", err)
			continue
		}
		if t > 0 {
			*throughput = append(*throughput, rawThroughput{deltaTime: d, numCommands: t})
		}
	}
	return nil
}
