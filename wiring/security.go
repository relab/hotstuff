package wiring

import (
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
)

type Security struct {
	blockChain *blockchain.BlockChain
	cryptoImpl modules.CryptoBase
	certAuth   *certauth.CertAuthority
}

// NewSecurity returns a set of dependencies necessary for application security and integrity.
func NewSecurity(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,
	sender modules.Sender,
	cryptoName string,
	opts ...certauth.Option,
) (*Security, error) {
	blockChain := blockchain.New(
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
		blockChain: blockChain,
		cryptoImpl: cryptoImpl,
		certAuth: certauth.New(
			config,
			logger,
			blockChain,
			cryptoImpl,
			opts...,
		),
	}, nil
}

// BlockChain returns the blockchain instance.
func (s *Security) BlockChain() *blockchain.BlockChain {
	return s.blockChain
}

// CryptoImpl returns the crypto implementation.
func (s *Security) CryptoImpl() modules.CryptoBase {
	return s.cryptoImpl
}

// CertAuth returns the certificate authority.
func (s *Security) CertAuth() *certauth.CertAuthority {
	return s.certAuth
}
