package modules

import (
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/metrics/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DataLogger logs data in protobuf message format.
type DataLogger interface {
	Log(proto.Message)
}

type dataLogger struct {
	mod *Modules
	wr  *protostream.Writer
}

// NewDataLogger returns a new data logger that logs to the given writer.
func NewDataLogger(wr *protostream.Writer) DataLogger {
	return &dataLogger{wr: wr}
}

// InitModule initializes the data logger module.
func (dl *dataLogger) InitModule(mod *Modules) {
	dl.mod = mod
}

func (dl *dataLogger) Log(msg proto.Message) {
	var err error
	if _, ok := msg.(*types.Event); !ok {
		var any *anypb.Any
		any, err = anypb.New(msg)
		if err != nil {
			dl.mod.Logger().Errorf("failed to create Any message: %v", err)
			return
		}
		event := &types.Event{
			ID:        uint32(dl.mod.ID()),
			Timestamp: timestamppb.Now(),
			Data:      any,
		}
		err = dl.wr.Write(event)
	} else {
		err = dl.wr.Write(msg)
	}
	if err != nil {
		dl.mod.Logger().Errorf("failed to write message to log: %v", err)
	}
}

type nopLogger struct{}

func (nopLogger) Log(proto.Message) {}

// NopLogger returns a logger that does not log anything.
// This is useful for testing and other situations where data logging is disabled.
func NopLogger() DataLogger {
	return nopLogger{}
}
