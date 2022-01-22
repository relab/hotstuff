package crypto

import "github.com/relab/hotstuff"

// Bitfield is an IDSet implemented by a bitfield. To check if an ID 'i' is present in the set, we simply check
// if the bit at i-1 is set (because IDs start at 1). This scales poorly if IDs are not sequential.
type Bitfield []byte

func (bm *Bitfield) extend(nBytes int) {
	// not sure if this is the most efficient way, but it was suggested here:
	// https://github.com/golang/go/wiki/SliceTricks#extend
	*bm = append(*bm, make([]byte, nBytes)...)
}

func (bm *Bitfield) set(byteIdx, bitIdx int) {
	(*bm)[byteIdx] |= 1 << bitIdx
}

func (bm *Bitfield) isSet(byteIdx, bitIdx int) bool {
	return (*bm)[byteIdx]&(1<<bitIdx) != 0
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

// Add adds an ID to the set.
func (bm *Bitfield) Add(id hotstuff.ID) {
	byteIdx, bitIdx := index(id)
	if len(*bm) <= byteIdx {
		bm.extend(byteIdx + 1 - len(*bm))
	}
	bm.set(byteIdx, bitIdx)
}

// Contains returns true if the set contains the ID.
func (bm *Bitfield) Contains(id hotstuff.ID) bool {
	byteIdx, bitIdx := index(id)
	if len(*bm) <= byteIdx {
		return false
	}
	return bm.isSet(byteIdx, bitIdx)
}

// ForEach calls f for each ID in the set.
func (bm *Bitfield) ForEach(f func(hotstuff.ID)) {
	for byteIdx := range *bm {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			if bm.isSet(byteIdx, bitIdx) {
				f(id(byteIdx, bitIdx))
			}
		}
	}
}

// Len returns the number of entries in the set.
func (bm *Bitfield) Len() int {
	// TODO: slow
	l := 0
	bm.ForEach(func(_ hotstuff.ID) { l++ })
	return l
}
