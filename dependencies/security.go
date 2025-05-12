package dependencies

import (
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
)

type Security struct {
	BlockChain *blockchain.BlockChain
	CryptoImpl modules.CryptoBase
	CertAuth   *certauth.CertAuthority
}

func NewSecurity(
	depsCore *Core,
	depsNet *Network,
	cryptoName string,
	opts ...certauth.Option,
) (*Security, error) {
	blockChain := blockchain.New(
		depsNet.Sender,
		depsCore.EventLoop(),
		depsCore.Logger(),
	)
	cryptoImpl, err := newCryptoModule(
		cryptoName,
		depsNet.Config,
		depsCore.Logger(),
		depsCore.Globals(),
	)
	if err != nil {
		return nil, err
	}
	return &Security{
		BlockChain: blockChain,
		CryptoImpl: cryptoImpl,
		CertAuth: certauth.New(
			cryptoImpl,
			blockChain,
			depsCore.Logger(),
			opts...,
		),
	}, nil
}
