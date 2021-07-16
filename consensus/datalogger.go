package consensus

import (
	"github.com/relab/hotstuff/internal/protostream"
	"google.golang.org/protobuf/proto"
)

// DataLogger logs data in protobuf message format.
type DataLogger interface {
	Log(proto.Message) error
	Close() error
}

type dataLogger struct {
	wr protostream.Writer
}

func (dl dataLogger) Log(msg proto.Message) error {
	return dl.wr.Write(msg)
}

func (dl dataLogger) Close() error {
	return dl.wr.Close()
}

type nopLogger struct{}

func (nopLogger) Log(proto.Message) error { return nil }
func (nopLogger) Close() error            { return nil }

// NopLogger returns a logger that does not log anything.
// This is useful for testing and other situations where data logging is disabled.
func NopLogger() DataLogger {
	return nopLogger{}
}
