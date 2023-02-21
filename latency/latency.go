// Package latency implements protocol latency measurements
package latency

import (
	"math"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("latencymeasurement", NewLatencyMeasurement)
}

// NewLatencyMeasurement initializes the latency measurement.
func NewLatencyMeasurement() modules.LatencyMeasurement {
	return &latencyMeasurement{latencyStats: make(map[hotstuff.ID]map[hotstuff.ID]*statInfo)}
}

type latencyMeasurement struct {
	configuration modules.Configuration
	opts          *modules.Options
	logger        logging.Logger
	eventLoop     *eventloop.EventLoop
	latencyStats  map[hotstuff.ID]map[hotstuff.ID]*statInfo
	isInitDone    bool
}
type statInfo struct {
	count    uint64
	mean     float64
	m2       float64
	retValue uint64
}

func (l *latencyMeasurement) InitModule(mods *modules.Core) {
	mods.Get(
		&l.configuration,
		&l.opts,
		&l.logger,
		&l.eventLoop,
	)
	l.eventLoop.RegisterHandler(hotstuff.LatencyVectorEvent{}, func(event any) {
		l.handleNewMeasurement(event.(hotstuff.LatencyVectorEvent))
	})
	l.eventLoop.RegisterObserver(backend.ConnectedEvent{}, func(_ any) {
		l.initialize()
	})
}

func (l *latencyMeasurement) initialize() {
	config := l.configuration.Replicas()
	for id := range config {
		tempStats := make(map[hotstuff.ID]*statInfo)
		for id1 := range config {
			tempStats[id1] = &statInfo{}
		}
		l.latencyStats[id] = tempStats
	}
	l.logger.Infof("stat are %v\n ", l.latencyStats)
	l.isInitDone = true
}

func (l *latencyMeasurement) handleNewMeasurement(event hotstuff.LatencyVectorEvent) {

	if !l.opts.IsLatencyCalculationEnabled() || !l.isInitDone {
		return
	}
	latencyVector := event.LatencyVector
	for id, newValue := range latencyVector {
		stats := l.latencyStats[event.Creator][hotstuff.ID(id)]
		if stats == nil {
			return
		}
		l.ComputeStats(l.latencyStats[event.Creator][hotstuff.ID(id)], float64(newValue))
	}
}

func (l *latencyMeasurement) GetLatencyMatrix() map[hotstuff.ID]map[hotstuff.ID]uint64 {
	latencyMatrix := make(map[hotstuff.ID]map[hotstuff.ID]uint64)
	if !l.opts.IsLatencyCalculationEnabled() || !l.isInitDone {
		return latencyMatrix
	}
	for id, latencyVector := range l.latencyStats {
		latencyMatrix[id] = make(map[hotstuff.ID]uint64)
		var max uint64
		for _, stats := range latencyVector {
			if stats.retValue > max {
				max = stats.retValue
			}
		}
		for id1, stats := range latencyVector {
			tmp := stats.retValue
			if stats.retValue == 0 {
				tmp = max
			}
			latencyMatrix[id][id1] = tmp
		}
	}
	return latencyMatrix
}

func (l *latencyMeasurement) ComputeStats(stats *statInfo, newValue float64) {
	stats.count++
	oldMean := stats.mean
	stats.mean += (newValue - oldMean) / float64(stats.count)
	stats.m2 += (newValue - oldMean) * (newValue - stats.mean)
	if stats.count > 1 {
		conf := 1.96
		dev := math.Sqrt(stats.m2 / float64(stats.count-1))
		stats.retValue = uint64(stats.mean) + uint64(dev*conf)
	}
}
