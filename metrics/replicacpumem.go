package metrics

import (
	"time"

	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// CPUMem metics measures the percentage of cpu and memory utilization on the node.
// If multiple replicas are run on the same node, then the data may be duplicated.
// This is not enabled by default, to enable this metric add "cpumem" string to --metrics option.
// Since it can interfere with the performance of the protocol, do not enable this metics unless required.
// Interval for measuring the cpu and memory utilization should be above 100 milliseconds, for valid data collection.
// This limitation is due to the gopsutil package.
func init() {
	RegisterReplicaMetric("cpumem", func() interface{} {
		return &CPUMemStat{}
	})
	RegisterClientMetric("cpumem", func() interface{} {
		return &CPUMemStat{}
	})
}

// CPUMemStat measures CPU usage and Memory usage and record in the metric logs.
type CPUMemStat struct {
	mods *modules.Modules
}

// InitModule gives the module access to the other modules.
func (c *CPUMemStat) InitModule(mods *modules.Modules) {
	c.mods = mods
	c.mods.EventLoop().RegisterObserver(types.TickEvent{}, func(event interface{}) {
		c.tick(event.(types.TickEvent))
	})
	c.mods.Logger().Info("CPU-Memory stats metric enabled")
	// Percent with 0 interval returns 0 usage when called first time.
	_, err := cpu.Percent(0, false)
	if err != nil {
		c.mods.Logger().Info("Unable to fetch the CPU usage")
	}
}

// getCPUsage Method returns the average CPU per core and the number of cores, including logical ones.
func (c *CPUMemStat) getCPUsage() (float64, uint32) {
	// Counts return the number of cores as our bbchain cluster has hyper-threading enabled,
	// logical parameter is set to true.
	cores, err := cpu.Counts(true)
	if err != nil {
		return 0, 0
	}
	usage, err := cpu.Percent(0, false)
	if err != nil {
		return 0, uint32(cores)
	}
	return usage[0], uint32(cores)
}

// getMemoryPercentage returns total memory available on the node and the currently utilized percentage.
func (c *CPUMemStat) getMemoryPercentage() (uint64, float64) {
	v, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0
	}
	return v.Available, v.UsedPercent
}

// tick method is invoked periodically based on the configured measuring interval of metrics
func (c *CPUMemStat) tick(_ types.TickEvent) {
	now := time.Now()
	cpusage, cores := c.getCPUsage()
	availablemem, memusage := c.getMemoryPercentage()
	event := &types.CPUMemoryStats{
		Event:                 types.NewReplicaEvent(uint32(c.mods.ID()), now),
		CPUsagePercentage:     cpusage,
		Cores:                 uint32(cores),
		MemoryUsagePercentage: memusage,
		AvailableMemory:       availablemem,
	}
	c.mods.MetricsLogger().Log(event)
}
