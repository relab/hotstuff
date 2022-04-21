package metrics

import (
	"time"

	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/metrics/types"
	"github.com/relab/hotstuff/modules"
	"google.golang.org/protobuf/types/known/durationpb"
)

func init() {
	RegisterReplicaMetric("throughput", func() interface{} {
		return &Throughput{}
	})
}

// Throughput measures throughput in commits per second, and commands per second.
type Throughput struct {
	mods         *modules.Modules
	commitCount  uint64
	commandCount uint64
}

// InitModule gives the module access to the other modules.
func (t *Throughput) InitModule(mods *modules.Modules) {
	t.mods = mods
	t.mods.EventLoop().RegisterHandler(consensus.CommitEvent{}, func(event interface{}) {
		commitEvent := event.(consensus.CommitEvent)
		t.recordCommit(commitEvent.Commands)
	})
	t.mods.EventLoop().RegisterObserver(types.TickEvent{}, func(event interface{}) {
		t.tick(event.(types.TickEvent))
	})
	t.mods.Logger().Info("Throughput metric enabled")
}

func (t *Throughput) recordCommit(commands int) {
	t.commitCount++
	t.commandCount += uint64(commands)
}

func (t *Throughput) tick(tick types.TickEvent) {
	now := time.Now()
	event := &types.ThroughputMeasurement{
		Event:    types.NewReplicaEvent(uint32(t.mods.ID()), now),
		Commits:  t.commitCount,
		Commands: t.commandCount,
		Duration: durationpb.New(now.Sub(tick.LastTick)),
	}
	t.mods.MetricsLogger().Log(event)
	// reset count for next tick
	t.commandCount = 0
	t.commitCount = 0
}
