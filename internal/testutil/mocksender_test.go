package testutil_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/security/crypto"
)

func TestPropose(t *testing.T) {
	r := testutil.WireUpEssentials(t, 1, crypto.NameECDSA)
	block := testutil.CreateBlock(t, r.Authority())
	r.MockSender().Propose(&hotstuff.ProposeMsg{
		ID:    1,
		Block: block,
	})
	// check if a message was sent at all
	if len(r.MockSender().MessagesSent()) != 1 {
		t.Error("message not sent")
	}
	// check if it was the correct type of message
	msg, ok := r.MockSender().MessagesSent()[0].(hotstuff.ProposeMsg)
	if !ok {
		t.Error("incorrect message type")
	}
	// below statements compare the data in the message
	if msg.ID != 1 {
		t.Error("incorrect sender")
	}
	if !bytes.Equal(block.ToBytes(), msg.Block.ToBytes()) {
		t.Error("incorrect data")
	}
}

func TestVote(t *testing.T) {
	r := testutil.WireUpEssentials(t, 1, crypto.NameECDSA)
	block := testutil.CreateBlock(t, r.Authority())
	pc := testutil.CreatePC(t, block, r.Authority())
	err := r.MockSender().Vote(2, pc)
	if err != nil {
		t.Fatal(err)
	}
	// check if a message was sent at all
	if len(r.MockSender().MessagesSent()) != 1 {
		t.Error("message not sent")
	}
	// check if it was the correct type of message
	msg, ok := r.MockSender().MessagesSent()[0].(hotstuff.PartialCert)
	if !ok {
		t.Error("incorrect message type")
	}
	// below statements compare the data in the message
	if msg.Signer() != 1 {
		t.Error("incorrect MockSender()")
	}

	if !bytes.Equal(msg.ToBytes(), pc.ToBytes()) {
		t.Error("incorrect data")
	}
}

func TestTimeout(t *testing.T) {
	r := testutil.WireUpEssentials(t, 1, crypto.NameECDSA)
	r.MockSender().Timeout(hotstuff.TimeoutMsg{
		ID:   1,
		View: 1,
	})
	// check if a message was sent at all
	if len(r.MockSender().MessagesSent()) != 1 {
		t.Error("message not sent")
	}
	// check if it was the correct type of message
	msg, ok := r.MockSender().MessagesSent()[0].(hotstuff.TimeoutMsg)
	if !ok {
		t.Error("incorrect message type")
	}
	// below statements compare the data in the message
	if msg.ID != 1 {
		t.Error("incorrect MockSender()")
	}

	if msg.View != 1 {
		t.Error("incorrect view")
	}
}

func TestSub(t *testing.T) {
	sender := testutil.NewMockSender(1, 2, 3, 4)
	var err error
	_, err = sender.Sub([]hotstuff.ID{2, 3})
	if err != nil {
		t.Fatal(err)
	}

	_, err = sender.Sub([]hotstuff.ID{5, 6})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestRequestBlock(t *testing.T) {
	set := testutil.NewEssentialsSet(t, 4, crypto.NameECDSA)
	first := set[0]
	second := set[1]
	block := testutil.CreateBlock(t, second.Authority())
	second.BlockChain().Store(block)

	_, ok := first.MockSender().RequestBlock(context.TODO(), block.Hash())
	if !ok {
		t.Fatal("expected a block to be returned")
	}
}
