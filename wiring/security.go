package wiring

import (
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
)

type Security struct {
	blockchain *blockchain.Blockchain
	cryptoImpl modules.CryptoBase
	auth       *cert.Authority
}

// NewSecurity returns a set of dependencies necessary for application security and integrity.
func NewSecurity(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,
	sender modules.Sender,
	cryptoName string,
	opts ...cert.Option,
) (*Security, error) {
	blockchain := blockchain.New(
		eventLoop,
		logger,
		sender,
	)
	cryptoImpl, err := newCryptoModule(
		logger,
		config,
		cryptoName,
	)
	if err != nil {
		return nil, err
	}
	return &Security{
		blockchain: blockchain,
		cryptoImpl: cryptoImpl,
		auth: cert.NewAuthority(
			config,
			blockchain,
			cryptoImpl,
			opts...,
		),
	}, nil
}

// BlockChain returns the blockchain instance.
func (s *Security) BlockChain() *blockchain.Blockchain {
	return s.blockchain
}

// CryptoImpl returns the crypto implementation.
func (s *Security) CryptoImpl() modules.CryptoBase {
	return s.cryptoImpl
}

// auth returns the certificate authority.
func (s *Security) Authority() *cert.Authority {
	return s.auth
}
