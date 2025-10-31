package clientpb

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

// Marshal marshals the Batch into a byte slice using protobuf.
// It panics if marshaling fails.
func (b *Batch) Marshal() []byte {
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(b)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal command batch: %v", err))
	}
	return data
}

// isFull returns true if the batch contains the specified number of commands.
func (b *Batch) isFull(batchSize uint32) bool {
	return uint32(len(b.Commands)) == batchSize
}
