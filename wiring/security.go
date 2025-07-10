package wiring

import (
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto"
)

type Security struct {
	blockchain *blockchain.Blockchain
	auth       *cert.Authority
}

// NewSecurity returns a set of dependencies necessary for application security and integrity.
func NewSecurity(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,
	sender core.Sender,
	base crypto.Base,
	opts ...cert.Option,
) *Security {
	blockchain := blockchain.New(
		eventLoop,
		logger,
		sender,
	)
	auth := cert.NewAuthority(
		config,
		blockchain,
		base,
		opts...,
	)
	return &Security{
		blockchain: blockchain,
		auth:       auth,
	}
}

// Blockchain returns the blockchain instance.
func (s *Security) Blockchain() *blockchain.Blockchain {
	return s.blockchain
}

// auth returns the certificate authority.
func (s *Security) Authority() *cert.Authority {
	return s.auth
}
