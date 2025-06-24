package cert

import (
	"fmt"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/security/crypto"
	"github.com/relab/hotstuff/security/crypto/bls12"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/security/crypto/eddsa"
)

func newCryptoModule(
	config *core.RuntimeConfig,
	name string,
) (impl crypto.Base, err error) {
	switch name {
	case "":
		fallthrough // default to ecdsa if no name is provided
	case ecdsa.ModuleName:
		impl = ecdsa.New(config)
	case bls12.ModuleName:
		impl, err = bls12.New(config)
		if err != nil {
			return nil, err
		}
	case eddsa.ModuleName:
		impl = eddsa.New(config)
	default:
		return nil, fmt.Errorf("invalid crypto name: '%s'", name)
	}
	return
}
