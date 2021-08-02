package modules

import (
	"fmt"
	"io"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// MetricsLogger logs data in protobuf message format.
type MetricsLogger interface {
	Log(proto.Message)
	io.Closer
}

type jsonLogger struct {
	mut   sync.Mutex
	mods  *Modules
	wr    io.Writer
	first bool
}

// NewJSONLogger returns a new metrics logger that logs to the specified writer.
func NewJSONLogger(wr io.Writer) (MetricsLogger, error) {
	_, err := io.WriteString(wr, "[\n")
	if err != nil {
		return nil, fmt.Errorf("failed to write start of JSON array: %v", err)
	}
	return &jsonLogger{wr: wr, first: true}, nil
}

// InitModule initializes the metrics logger module.
func (dl *jsonLogger) InitModule(mods *Modules) {
	dl.mods = mods
}

func (dl *jsonLogger) Log(msg proto.Message) {
	var (
		any *anypb.Any
		err error
		ok  bool
	)
	if any, ok = msg.(*anypb.Any); !ok {
		any, err = anypb.New(msg)
		if err != nil {
			dl.mods.Logger().Errorf("failed to create Any message: %v", err)
			return
		}
	}
	err = dl.write(any)
	if err != nil {
		dl.mods.Logger().Errorf("failed to write message to log: %v", err)
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
func NopLogger() MetricsLogger {
	return nopLogger{}
}
