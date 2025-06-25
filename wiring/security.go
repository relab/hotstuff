package wiring

import (
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
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
	cryptoName string,
	opts ...cert.Option,
) (*Security, error) {
	blockchain := blockchain.New(
		eventLoop,
		logger,
		sender,
	)
	auth, err := cert.NewAuthority(
		config,
		blockchain,
		cryptoName,
		opts...,
	)
	if err != nil {
		return nil, err
	}
	return &Security{
		blockchain: blockchain,
		auth:       auth,
	}, nil
}

// BlockChain returns the blockchain instance.
func (s *Security) BlockChain() *blockchain.Blockchain {
	return s.blockchain
}

// auth returns the certificate authority.
func (s *Security) Authority() *cert.Authority {
	return s.auth
}
