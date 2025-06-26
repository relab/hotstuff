package crypto

import (
	"fmt"

	"github.com/relab/hotstuff/core"
)

func New(
	config *core.RuntimeConfig,
	name string,
) (impl Base, err error) {
	switch name {
	case "":
		fallthrough // default to ecdsa if no name is provided
	case NameECDSA:
		impl = NewECDSA(config)
	case NameEDDSA:
		impl = NewEDDSA(config)
	case NameBLS12:
		impl, err = NewBLS12(config)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid crypto name: '%s'", name)
	}
	return
}
