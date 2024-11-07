package pipeline

import "encoding/binary"

type Pipe uint32

const NullPipe = Pipe(0)

// ToBytes returns the pipe as bytes.
func (p Pipe) ToBytes() []byte {
	var viewBytes [4]byte
	binary.LittleEndian.PutUint32(viewBytes[:], uint32(p))
	return viewBytes[:]
}

// If the pipe ID is not NullPipe, then return true
func ValidPipe(pipeId Pipe) bool {
	return pipeId != NullPipe
}

// If the list contains duplicate IDs or a NullPipeId, then return false
// func ValidPipes(pipeIds []Pipe) bool {
// 	for i, pId := range pipeIds {
// 		if !ValidPipe(pId) {
// 			return false
// 		}
//
// 		for j, otherPid := range pipeIds {
// 			if i != j && otherPid == pId {
// 				return false
// 			}
// 		}
// 	}
// 	return true
// }
