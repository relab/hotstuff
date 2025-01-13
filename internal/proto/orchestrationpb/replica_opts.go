package orchestrationpb

import (
	"strconv"
	"strings"
	"time"

	"github.com/relab/hotstuff"
	"google.golang.org/protobuf/proto"
)

// New creates a new ReplicaOpts with the given replicaID and locations.
// The locations are shared between all replicas, and are indexed by the replicaID.
func (x *ReplicaOpts) New(replicaID hotstuff.ID, locations []string) *ReplicaOpts {
	replicaOpts := proto.Clone(x).(*ReplicaOpts)
	replicaOpts.ID = uint32(replicaID)
	replicaOpts.Locations = locations
	return replicaOpts
}

// TreePositionIDs returns the tree positions as a slice of hotstuff.ID.
func (x *ReplicaOpts) TreePositionIDs() []hotstuff.ID {
	ids := make([]hotstuff.ID, len(x.GetTreePositions()))
	for i, id := range x.GetTreePositions() {
		ids[i] = hotstuff.ID(id)
	}
	return ids
}

// TreeDeltaDuration returns the tree delta as a time.Duration.
func (x *ReplicaOpts) TreeDeltaDuration() time.Duration {
	return x.GetTreeDelta().AsDuration()
}

func (x *ReplicaOpts) SetTreePositions(positions []uint32) {
	x.TreePositions = positions
}

func (x *ReplicaOpts) SetByzantineStrategy(strategy string) {
	x.ByzantineStrategy = strategy
}

func (x *ReplicaOpts) HotstuffID() hotstuff.ID {
	return hotstuff.ID(x.GetID())
}

func (x *ReplicaOpts) StringID() string {
	return strconv.Itoa(int(x.GetID()))
}

func (x *ReplicaOpts) StringLocations() string {
	s := strings.Builder{}
	s.WriteString("ID: ")
	s.WriteString(x.StringID())
	s.WriteString(", ")
	s.WriteString("Locations: ")
	s.WriteString(strings.Join(x.GetLocations(), ", "))
	return s.String()
}
