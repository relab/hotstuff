package testutil_test

import (
	"bytes"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/wiring"
)

type replica struct {
	logger     logging.Logger
	eventLoop  *eventloop.EventLoop
	config     *core.RuntimeConfig
	sender     *testutil.MockSender
	blockChain *blockchain.BlockChain
	auth       *cert.Authority
}

func wireUpReplica(t *testing.T) *replica {
	logger := logging.New("test")
	eventLoop := eventloop.New(logger, 1)
	config := core.NewRuntimeConfig(1, testutil.GenerateECDSAKey(t))

	sender := testutil.NewMockSender(1)
	chain := blockchain.New(eventLoop, logger, sender)
	auth := cert.NewAuthority(config, chain, ecdsa.New(config))
	return &replica{
		logger:     logger,
		eventLoop:  eventLoop,
		config:     config,
		sender:     sender,
		blockChain: chain,
		auth:       auth,
	}
}

func TestPropose(t *testing.T) {
	r := wireUpReplica(t)
	block := testutil.CreateBlock(t, r.auth)
	r.sender.Propose(&hotstuff.ProposeMsg{
		ID:    1,
		Block: block,
	})
	// check if a message was sent at all
	if len(r.sender.MessagesSent()) != 1 {
		t.Error("message not sent")
	}
	// check if it was the correct type of message
	msg, ok := r.sender.MessagesSent()[0].(hotstuff.ProposeMsg)
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
	r := wireUpReplica(t)
	block := testutil.CreateBlock(t, r.auth)
	pc := testutil.CreatePC(t, block, r.auth)
	r.sender.Vote(2, pc)
	// check if a message was sent at all
	if len(r.sender.MessagesSent()) != 1 {
		t.Error("message not sent")
	}
	// check if it was the correct type of message
	msg, ok := r.sender.MessagesSent()[0].(hotstuff.PartialCert)
	if !ok {
		t.Error("incorrect message type")
	}
	// below statements compare the data in the message
	if msg.Signer() != 1 {
		t.Error("incorrect sender")
	}

	if !bytes.Equal(msg.ToBytes(), pc.ToBytes()) {
		t.Error("incorrect data")
	}
}

func TestTimeout(t *testing.T) {
	r := wireUpReplica(t)
	r.sender.Timeout(hotstuff.TimeoutMsg{
		ID:   1,
		View: 1,
	})
	// check if a message was sent at all
	if len(r.sender.MessagesSent()) != 1 {
		t.Error("message not sent")
	}
	// check if it was the correct type of message
	msg, ok := r.sender.MessagesSent()[0].(hotstuff.TimeoutMsg)
	if !ok {
		t.Error("incorrect message type")
	}
	// below statements compare the data in the message
	if msg.ID != 1 {
		t.Error("incorrect sender")
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
	replicaIDs := []hotstuff.ID{1, 2, 3, 4}
	chains := make([]*blockchain.BlockChain, 0)
	senders := make([]*testutil.MockSender, 0)
	var firstCore *wiring.Core
	for _, id := range replicaIDs {
		depsCore := wiring.NewCore(id, "test", testutil.GenerateECDSAKey(t))
		if firstCore == nil {
			firstCore = depsCore
		}
		sender := testutil.NewMockSender(id, replicaIDs...)
		chain := blockchain.New(
			depsCore.EventLoop(),
			depsCore.Logger(),
			sender,
		)
		senders = append(senders, sender)
		chains = append(chains, chain)
	}
	for _, chain := range chains {
		for _, sender := range senders {
			sender.AddBlockChain(chain)
		}
	}
	signer := cert.NewAuthority(
		firstCore.RuntimeCfg(),
		chains[0],
		ecdsa.New(firstCore.RuntimeCfg()),
	)
	sender0 := senders[0]
	chain1 := chains[1]
	block := testutil.CreateBlock(t, signer)
	chain1.Store(block)

	_, ok := sender0.RequestBlock(nil, block.Hash())
	if !ok {
		t.Fatal("expected a block to be returned")
	}
}
