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

// Unmarshal unmarshals the byte slice into a Batch using protobuf.
// It panics if unmarshaling fails.
func Unmarshal(data []byte) *Batch {
	batch := new(Batch)
	err := proto.UnmarshalOptions{DiscardUnknown: true, AllowPartial: true}.Unmarshal(data, batch)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal command batch: %v", err))
	}
	return batch
}
