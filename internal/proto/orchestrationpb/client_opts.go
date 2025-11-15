package orchestrationpb

import (
	"strconv"
	"unsafe"

	"github.com/relab/hotstuff/client"
)

// ClientID returns the client's ID.
func (x *ClientOpts) ClientID() client.ID {
	return client.ID(x.GetID())
}

// ClientIDString returns the client's ID as a string.
func (x *ClientOpts) ClientIDString() string {
	return strconv.Itoa(int(x.GetID()))
}

// ClientIDs returns the list of client IDs.
func (x *StopClientRequest) ClientIDs() []client.ID {
	if x == nil || len(x.IDs) == 0 {
		return nil
	}
	return unsafe.Slice((*client.ID)(unsafe.Pointer(unsafe.SliceData(x.IDs))), len(x.IDs))
}
