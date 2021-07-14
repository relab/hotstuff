package datalogger_test

import (
	"bytes"
	"testing"

	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/datalogger"
	"github.com/relab/hotstuff/internal/proto/hotstuffpb"
)

func TestDataLogger(t *testing.T) {
	var buf bytes.Buffer                                   // in-memory log
	msg := hotstuffpb.BlockToProto(consensus.GetGenesis()) // test message

	logWriter := datalogger.NewWriter(&buf)
	logReader := datalogger.NewReader(&buf)

	err := logWriter.Log(msg)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	gotMsg, err := logReader.Read()
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
