package crypto

import (
	"github.com/relab/hotstuff"
)

// Bitfield is an IDSet implemented by a bitfield. To check if an ID 'i' is present in the set, we simply check
// if the bit at i-1 is set (because IDs start at 1). This scales poorly if IDs are not sequential.
type Bitfield struct {
	data []byte
	len  int
}

func (bf *Bitfield) extend(nBytes int) {
	// not sure if this is the most efficient way, but it was suggested here:
	// https://github.com/golang/go/wiki/SliceTricks#extend
	bf.data = append(bf.data, make([]byte, nBytes)...)
}

func (bf *Bitfield) set(byteIdx, bitIdx int) {
	if !bf.isSet(byteIdx, bitIdx) {
		bf.len++
	}
	bf.data[byteIdx] |= 1 << bitIdx
}

func (bf Bitfield) isSet(byteIdx, bitIdx int) bool {
	return bf.data[byteIdx]&(1<<bitIdx) != 0
}

// index returns the byte index and the bit index to use based on the id.
func index(id hotstuff.ID) (byteIdx, bitIdx int) {
	i := int(id) - 1
	byteIdx = i / 8
	bitIdx = i % 8
	return
}

func id(byteIdx, bitIdx int) hotstuff.ID {
	return hotstuff.ID(1 + (byteIdx * 8) + bitIdx)
}

// BitfieldFromBytes creates a bitfield from the given byte slice.
func BitfieldFromBytes(b []byte) Bitfield {
	bf := Bitfield{
		data: b,
		len:  0,
	}
	l := 0
	bf.ForEach(func(i hotstuff.ID) {
		l++
	})
	bf.len = l
	return bf
}

// Bytes returns the raw byte slice containing the data of this bitfield.
func (bf Bitfield) Bytes() []byte {
	return bf.data
}

// Add adds an ID to the set.
func (bf *Bitfield) Add(id hotstuff.ID) {
	byteIdx, bitIdx := index(id)
	if len(bf.data) <= byteIdx {
		bf.extend(byteIdx + 1 - len(bf.data))
	}
	bf.set(byteIdx, bitIdx)
}

// Contains returns true if the set contains the ID.
func (bf Bitfield) Contains(id hotstuff.ID) bool {
	byteIdx, bitIdx := index(id)
	if len(bf.data) <= byteIdx {
		return false
	}
	return bf.isSet(byteIdx, bitIdx)
}

// ForEach calls f for each ID in the set.
func (bf Bitfield) ForEach(f func(hotstuff.ID)) {
	bf.RangeWhile(func(i hotstuff.ID) bool {
		f(i)
		return true
	})
}

// RangeWhile calls f for each ID in the set until f returns false.
func (bf Bitfield) RangeWhile(f func(hotstuff.ID) bool) {
	for byteIdx := range bf.data {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			if bf.isSet(byteIdx, bitIdx) {
				if !f(id(byteIdx, bitIdx)) {
					return
				}
			}
		}
	}
}

// Len returns the number of entries in the set.
func (bf Bitfield) Len() int {
	return bf.len
}

func (bf Bitfield) String() string {
	return hotstuff.IDSetToString(&bf)
}
