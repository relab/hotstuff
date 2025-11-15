package orchestrationpb

import (
	"github.com/relab/hotstuff/client"
)

// ClientID returns the client's ID.
func (x *ClientOpts) ClientID() client.ID {
	return client.ID(x.GetID())
}
