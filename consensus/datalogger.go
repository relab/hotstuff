package consensus

import (
	"github.com/relab/hotstuff/internal/protostream"
	"google.golang.org/protobuf/proto"
)

// DataLogger logs data in protobuf message format.
type DataLogger interface {
	Log(proto.Message) error
}

type dataLogger struct {
	wr *protostream.Writer
}

// NewDataLogger returns a new data logger that logs to the given writer.
func NewDataLogger(wr *protostream.Writer) DataLogger {
	return &dataLogger{wr}
}

func (dl dataLogger) Log(msg proto.Message) error {
	return dl.wr.WriteAny(msg)
}

type nopLogger struct{}

func (nopLogger) Log(proto.Message) error { return nil }

// NopLogger returns a logger that does not log anything.
// This is useful for testing and other situations where data logging is disabled.
func NopLogger() DataLogger {
	return nopLogger{}
}
