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

// IDSlice is a slice containing IDs.
type IDSlice []ID

// Uint32Slice converts the IDSlice to a []uint32.
// Useful for passing into protocol buffer types.
func (ids IDSlice) Uint32Slice() []uint32 {
	converted := make([]uint32, 0, len(ids))
	for _, id := range ids {
		converted = append(converted, uint32(id))
	}
	return converted
}
