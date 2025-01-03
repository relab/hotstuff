package orchestrationpb

import (
	"strconv"
	"strings"

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
