package protostream_test

import (
	"bytes"
	"testing"

	"github.com/relab/hotstuff/msg"

	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/protostream"
)

func TestProtostream(t *testing.T) {
	var buf bytes.Buffer                                // in-memory stream
	genMsg := hotstuffpb.BlockToProto(msg.GetGenesis()) // test message

	writer := protostream.NewWriter(&buf)
	reader := protostream.NewReader(&buf)

	err := writer.WriteAny(genMsg)
	if err != nil {
		t.Fatalf("WriteAny failed: %v", err)
	}

	gotMsg, err := reader.ReadAny()
	if err != nil {
		t.Fatalf("ReadAny failed: %v", err)
	}

	got, ok := gotMsg.(*hotstuffpb.Block)
	if !ok {
		t.Fatalf("wrong message type returned: got: %T, want: %T", got, genMsg)
	}

	gotBlock := hotstuffpb.BlockFromProto(got)
	if gotBlock.Hash() != msg.GetGenesis().Hash() {
		t.Fatalf("message hash did not match")
	}
}
