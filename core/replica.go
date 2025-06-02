package core

import (
	"fmt"

	"github.com/relab/hotstuff"
)

// ReplicaInfo returns a replica if it is present in the configuration.
func (g *RuntimeConfig) ReplicaInfo(id hotstuff.ID) (replica *hotstuff.ReplicaInfo, ok bool) {
	replica, ok = g.replicas[id]
	return
}

// ReplicaCount returns the number of replicas in the configuration.
func (g *RuntimeConfig) ReplicaCount() int {
	return len(g.replicas)
}

// QuorumSize returns the size of a quorum.
func (g *RuntimeConfig) QuorumSize() int {
	return hotstuff.QuorumSize(g.ReplicaCount())
}

// AddReplica adds information about the replica.
func (g *RuntimeConfig) AddReplica(replicaInfo *hotstuff.ReplicaInfo) {
	g.replicas[replicaInfo.ID] = replicaInfo
}

// SetReplicaMetadata sets the metadata for a replica based on id.
func (g *RuntimeConfig) SetReplicaMetadata(id hotstuff.ID, metadata map[string]string) error {
	if _, ok := g.replicas[id]; !ok {
		return fmt.Errorf("replica %d does not exist", id)
	}
	g.replicas[id].Metadata = metadata
	return nil
}

// AddConnectionMetadata sets the value of a key in the connection metadata map.
//
// NOTE: if the value contains binary data, the key must have the "-bin" suffix.
// This is to make it compatible with GRPC metadata.
// See: https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md#storing-binary-data-in-metadata
func (g *RuntimeConfig) AddConnectionMetadata(key string, value string) {
	g.connectionMetadata[key] = value
}

// ConnectionMetadata returns the metadata map that is sent when connecting to other replicas.
func (g *RuntimeConfig) ConnectionMetadata() map[string]string {
	return g.connectionMetadata
}
