package modules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// DataLogger logs data in protobuf message format.
type DataLogger interface {
	Log(proto.Message)
	io.Closer
}

type jsonDataLogger struct {
	mut   sync.Mutex
	mods  *Modules
	wr    io.Writer
	buf   bytes.Buffer
	first bool
}

// NewJSONDataLogger returns a new data logger that logs to the given writer.
func NewJSONDataLogger(wr io.Writer) (DataLogger, error) {
	_, err := io.WriteString(wr, "[\n")
	if err != nil {
		return nil, fmt.Errorf("failed to write start of JSON array: %v", err)
	}
	return &jsonDataLogger{wr: wr, first: true}, nil
}

// InitModule initializes the data logger module.
func (dl *jsonDataLogger) InitModule(mods *Modules) {
	dl.mods = mods
}

func (dl *jsonDataLogger) Log(msg proto.Message) {
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

func (dl *jsonDataLogger) write(msg proto.Message) (err error) {
	dl.mut.Lock()
	defer dl.mut.Unlock()
	dl.buf.Reset()

	if dl.first {
		dl.first = false
	} else {
		// write a comma and newline to separate the messages
		_, err := io.WriteString(dl.wr, ",\n")
		if err != nil {
			return err
		}
	}

	b, err := protojson.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message to JSON: %w", err)
	}
	err = json.Indent(&dl.buf, b, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to indent JSON: %w", err)
	}
	_, err = io.Copy(dl.wr, &dl.buf)
	return err
}

// Close closes the data logger
func (dl *jsonDataLogger) Close() error {
	_, err := io.WriteString(dl.wr, "\n]")
	return err
}

type nopLogger struct{}

func (nopLogger) Log(proto.Message) {}
func (nopLogger) Close() error      { return nil }

// NopLogger returns a logger that does not log anything.
// This is useful for testing and other situations where data logging is disabled.
func NopLogger() DataLogger {
	return nopLogger{}
}
