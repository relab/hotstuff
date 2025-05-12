package dependencies

import (
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
)

type Security struct {
	blockChain *blockchain.BlockChain
	cryptoImpl modules.CryptoBase
	certAuth   *certauth.CertAuthority
}

func NewSecurity(
	depsCore *Core,
	depsNet *Network,
	cryptoName string,
	opts ...certauth.Option,
) (*Security, error) {
	blockChain := blockchain.New(
		depsNet.Sender(),
		depsCore.EventLoop(),
		depsCore.Logger(),
	)
	cryptoImpl, err := newCryptoModule(
		cryptoName,
		depsNet.Config(),
		depsCore.Logger(),
		depsCore.Globals(),
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
			depsCore.Logger(),
			opts...,
		),
	}, nil
}

// GetBlockChain returns the blockchain instance.
func (s *Security) BlockChain() *blockchain.BlockChain {
	return s.blockChain
}

// GetCryptoImpl returns the crypto implementation.
func (s *Security) CryptoImpl() modules.CryptoBase {
	return s.cryptoImpl
}

// GetCertAuth returns the certificate authority.
func (s *Security) CertAuth() *certauth.CertAuthority {
	return s.certAuth
}
