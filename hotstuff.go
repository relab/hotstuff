// Package hotstuff implements the basic types that are used by hotstuff.
package hotstuff

import "encoding/binary"

// ID uniquely identifies a replica
type ID uint32

// ToBytes returns the ID as bytes.
func (id ID) ToBytes() []byte {
	var idBytes [4]byte
	binary.LittleEndian.PutUint32(idBytes[:], uint32(id))
	return idBytes[:]
}

// DefaultLocation is the default location of a replica.
const DefaultLocation = "default"

// DefaultLatency is the default latencies between the default replicas.
const DefaultLatency = 500
