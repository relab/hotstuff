package handel

import (
	"errors"
	"math"
	"math/rand"
	"reflect"
	"sort"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/backend"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/handelpb"
)

type Handel struct {
	mods       *consensus.Modules
	cfg        *handelpb.Configuration
	partitions [][]hotstuff.ID
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (h *Handel) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	h.mods = mods
}

func (h *Handel) Init() error {
	var cfg *backend.Config
	var srv *backend.Server
	if !h.mods.GetModuleByType(&srv) {
		return errors.New("could not get gorums server")
	}
	if !h.mods.GetModuleByType(&cfg) {
		return errors.New("could not get gorums configuration")
	}

	handelpb.RegisterHandelServer(srv.GetGorumsServer(), serviceImpl{h})
	h.cfg = handelpb.ConfigurationFromRaw(cfg.GetRawConfiguration(), nil)

	rnd := rand.New(rand.NewSource(h.mods.Options().SharedRandomSeed()))
	order := make([]hotstuff.ID, 0, h.mods.Configuration().Len())
	for id := range h.mods.Configuration().Replicas() {
		order = append(order, id)
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	rnd.Shuffle(len(order), reflect.Swapper(order))
	h.mods.Logger().Debug("Handel order: %v", order)

	maxLevel := int(math.Ceil(math.Log2(float64(h.mods.Configuration().Len()))))
	h.mods.Logger().Infof("Handel: %d levels", maxLevel)

	h.partitions = createPartitions(maxLevel, order, h.mods.ID())
	h.mods.Logger().Infof("Handel partitions: %v", h.partitions)

	return nil
}

func createPartitions(levels int, order []hotstuff.ID, self hotstuff.ID) (partitions [][]hotstuff.ID) {
	partitions = make([][]hotstuff.ID, levels+1)

	selfIndex := -1
	for i, id := range order {
		if id == self {
			selfIndex = i
			break
		}
	}

	curLevel := levels
	l := 0
	r := len(order) - 1
	for curLevel > 0 {
		m := int(math.Ceil(float64(l+r) / 2))
		if m < int(selfIndex) { // pick right side
			partitions[curLevel] = order[l:m]
			l = m + 1
		} else if m >= int(selfIndex) { // pick left side
			partitions[curLevel] = order[m : r+1]
			r = m - 1
		}
		curLevel--
	}

	return partitions
}

type serviceImpl struct {
	h *Handel
}

func (impl serviceImpl) Contribute(ctx gorums.ServerCtx, msg *handelpb.Contribution) {

}
