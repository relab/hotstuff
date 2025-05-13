package core

import "github.com/relab/hotstuff"

// ReplicaInfo returns a replica if it is present in the configuration.
func (g *RuntimeConfig) ReplicaInfo(id hotstuff.ID) (replica *hotstuff.ReplicaInfo, ok bool) {
	replica, ok = g.replicas[id]
	return
}

// ReplicaCount returns the number of replicas in the configuration.
func (g *RuntimeConfig) ReplicaCount() int {
	return len(g.replicas)
}

// QuorumSize returns the size of a quorum
func (g *RuntimeConfig) QuorumSize() int {
	return hotstuff.QuorumSize(g.ReplicaCount())
}

// AddReplica adds information about the replica.
func (g *RuntimeConfig) AddReplica(replicaInfo *hotstuff.ReplicaInfo) {
	g.replicas[replicaInfo.ID] = replicaInfo
}

// TODO(AlanRostem): can this be avoided by integrating it into SetConnectionMetadata?
func (g *RuntimeConfig) SetReplicaMetaData(id hotstuff.ID, metaData map[string]string) {
	g.replicas[id].MetaData = metaData
}

// SetConnectionMetadata sets the value of a key in the connection metadata map.
//
// NOTE: if the value contains binary data, the key must have the "-bin" suffix.
// This is to make it compatible with GRPC metadata.
// See: https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md#storing-binary-data-in-metadata
func (g *RuntimeConfig) SetConnectionMetadata(key string, value string) {
	g.connectionMetadata[key] = value
}

// ConnectionMetadata returns the metadata map that is sent when connecting to other replicas.
func (g *RuntimeConfig) ConnectionMetadata() map[string]string {
	return g.connectionMetadata
}
