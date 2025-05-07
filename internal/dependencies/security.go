package dependencies

import (
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/certauth"
)

type DepSetSecurity struct {
	BlockChain *blockchain.BlockChain
	CryptoImpl modules.CryptoBase
	CertAuth   *certauth.CertAuthority
}

func NewSecurity(
	coreComps *DepSetCore,
	netComps *DepSetNetwork,
	cryptoName string,
	cacheSize int,
) (*DepSetSecurity, error) {
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
	return &DepSetSecurity{
		BlockChain: blockChain,
		CryptoImpl: cryptoImpl,
		CertAuth:   certAuthority,
	}, nil
}
