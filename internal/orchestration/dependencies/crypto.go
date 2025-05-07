package dependencies

import (
	"fmt"

	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network/netconfig"
	"github.com/relab/hotstuff/security/crypto/bls12"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/security/crypto/eddsa"
)

func NewCryptoImpl(
	name string,
	configuration *netconfig.Config,
	logger logging.Logger,
	opts *core.Options,
) (impl modules.CryptoBase, err error) {
	switch name {
	case bls12.ModuleName:
		impl = bls12.New(configuration, logger, opts)
	case ecdsa.ModuleName:
		impl = ecdsa.New(configuration, logger, opts)
	case eddsa.ModuleName:
		impl = eddsa.New(configuration, logger, opts)
	default:
		return nil, fmt.Errorf("invalid crypto name: '%s'", name)
	}
	return
}
