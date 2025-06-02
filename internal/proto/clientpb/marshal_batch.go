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
