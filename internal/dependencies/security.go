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
	cacheSize int,
) (*Security, error) {
	blockChain := blockchain.New(
		depsNet.Sender,
		depsCore.EventLoop,
		depsCore.Logger,
	)
	cryptoImpl, err := newCryptoModule(cryptoName, depsNet.Config, depsCore.Logger, depsCore.Globals)
	if err != nil {
		return nil, err
	}
	var certAuthority *certauth.CertAuthority
	if cacheSize > 0 {
		certAuthority = certauth.NewCached(
			cryptoImpl,
			blockChain,
			depsCore.Logger,
			cacheSize,
		)
	} else {
		certAuthority = certauth.New(
			cryptoImpl,
			blockChain,
			depsCore.Logger,
		)
	}
	return &Security{
		BlockChain: blockChain,
		CryptoImpl: cryptoImpl,
		CertAuth:   certAuthority,
	}, nil
}
