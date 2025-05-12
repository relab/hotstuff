package dependencies

import (
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/globals"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network/netconfig"
	"github.com/relab/hotstuff/network/sender"
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
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	globals *globals.Globals,
	config *netconfig.Config,
	sender *sender.Sender,
	cryptoName string,
	opts ...certauth.Option,
) (*Security, error) {
	blockChain := blockchain.New(
		sender,
		eventLoop,
		logger,
	)
	cryptoImpl, err := newCryptoModule(
		cryptoName,
		config,
		logger,
		globals,
	)
	if err != nil {
		return nil, err
	}
	return &Security{
		blockChain: blockChain,
		cryptoImpl: cryptoImpl,
		certAuth: certauth.New(
			cryptoImpl,
			blockChain,
			logger,
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
