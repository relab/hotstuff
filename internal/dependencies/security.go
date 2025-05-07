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
	coreComps *Core,
	netComps *Network,
	cryptoName string,
	cacheSize int,
) (*Security, error) {
	blockChain := blockchain.New(
		netComps.Sender,
		coreComps.EventLoop,
		coreComps.Logger,
	)
	cryptoImpl, err := newCryptoModule(cryptoName, netComps.Config, coreComps.Logger, coreComps.Options)
	if err != nil {
		return nil, err
	}
	var certAuthority *certauth.CertAuthority
	if cacheSize > 0 {
		certAuthority = certauth.NewCached(
			cryptoImpl,
			blockChain,
			coreComps.Logger,
			cacheSize,
		)
	} else {
		certAuthority = certauth.New(
			cryptoImpl,
			blockChain,
			coreComps.Logger,
		)
	}
	return &Security{
		BlockChain: blockChain,
		CryptoImpl: cryptoImpl,
		CertAuth:   certAuthority,
	}, nil
}
