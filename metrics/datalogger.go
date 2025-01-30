package metrics

import (
	"fmt"
	"io"
	"sync"

	"github.com/relab/hotstuff/core/logging"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Logger logs data in protobuf message format.
type Logger interface {
	Log(proto.Message)
	io.Closer
}

type jsonLogger struct {
	logger logging.Logger

	mut   sync.Mutex
	wr    io.Writer
	first bool
}

// NewJSONLogger returns a new metrics logger that logs to the specified writer.
func NewJSONLogger(wr io.Writer, logger logging.Logger) (Logger, error) {
	_, err := io.WriteString(wr, "[\n")
	if err != nil {
		return nil, fmt.Errorf("failed to write start of JSON array: %v", err)
	}
	return &jsonLogger{logger: logger, wr: wr, first: true}, nil
}

func (dl *jsonLogger) Log(msg proto.Message) {
	var (
		anyMsg *anypb.Any
		err    error
		ok     bool
	)
	if anyMsg, ok = msg.(*anypb.Any); !ok {
		anyMsg, err = anypb.New(msg)
		if err != nil {
			dl.logger.Errorf("failed to create Any message: %v", err)
			return
		}
	}
	err = dl.write(anyMsg)
	if err != nil {
		dl.logger.Errorf("failed to write message to log: %v", err)
	}
}

func (dl *jsonLogger) write(msg proto.Message) (err error) {
	dl.mut.Lock()
	defer dl.mut.Unlock()

	if dl.first {
		dl.first = false
	} else {
		// write a comma and newline to separate the messages
		_, err := io.WriteString(dl.wr, ",\n")
		if err != nil {
			return err
		}
	}

	b, err := protojson.MarshalOptions{
		Indent:          "\t",
		EmitUnpopulated: true,
	}.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message to JSON: %w", err)
	}
	_, err = dl.wr.Write(b)
	return err
}

// Close closes the metrics logger
func (dl *jsonLogger) Close() error {
	_, err := io.WriteString(dl.wr, "\n]")
	return err
}

type nopLogger struct{}

func (nopLogger) Log(proto.Message) {}
func (nopLogger) Close() error      { return nil }

// NopLogger returns a metrics logger that discards any messages.
// This is useful for testing and other situations where metrics logging is disabled.
func NopLogger() Logger {
	return nopLogger{}
}
