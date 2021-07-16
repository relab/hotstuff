package protostream_test

import (
	"bytes"
	"testing"

	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
	"github.com/relab/hotstuff/internal/protostream"
)

func TestDataLogger(t *testing.T) {
	var buf bytes.Buffer                                   // in-memory stream
	msg := hotstuffpb.BlockToProto(consensus.GetGenesis()) // test message

	writer := protostream.NewWriter(&buf)
	reader := protostream.NewReader(&buf)

	err := writer.Write(msg)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	gotMsg, err := reader.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	got, ok := gotMsg.(*hotstuffpb.Block)
	if !ok {
		t.Fatalf("wrong message type returned: got: %T, want: %T", got, msg)
	}

	gotBlock := hotstuffpb.BlockFromProto(got)
	if gotBlock.Hash() != consensus.GetGenesis().Hash() {
		t.Fatalf("message hash did not match")
	}
}
