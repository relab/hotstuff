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
