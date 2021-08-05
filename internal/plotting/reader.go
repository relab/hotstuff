package plotting

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"reflect"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/anypb"
)

// Measurements is a list of measurements.
type Measurements interface {
	// Add adds a measurement to the list.
	Add(interface{})
}

// Reader reads measurements from JSON.
type Reader struct {
	plots map[reflect.Type]Measurements
	rd    io.Reader
}

// NewReader returns a new reader that reads from the specified source.
func NewReader(rd io.Reader) *Reader {
	return &Reader{
		plots: make(map[reflect.Type]Measurements),
		rd:    rd,
	}
}

// ReadAll reads all measurements in the source.
func (r *Reader) ReadAll() error {
	decoder := json.NewDecoder(r.rd)

	t, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("failed to read first JSON token: %w", err)
	}
	if d, ok := t.(json.Delim); !ok || d != '[' {
		return fmt.Errorf("expected first JSON token to be the start of an array")
	}

	for decoder.More() {
		var b json.RawMessage
		err = decoder.Decode(&b)
		if err != nil {
			return err
		}
		err = r.read(b)
		if err != nil {
			return err
		}
	}

	t, err = decoder.Token()
	if err != nil {
		return fmt.Errorf("failed to read last JSON token: %w", err)
	}
	if d, ok := t.(json.Delim); !ok || d != ']' {
		return fmt.Errorf("expected last JSON token to be the end of an array")
	}

	return nil
}

func (r *Reader) read(b []byte) error {
	any := &anypb.Any{}
	err := protojson.Unmarshal(b, any)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON message: %w", err)
	}

	msg, err := any.UnmarshalNew()
	if err != nil {
		return fmt.Errorf("failed to unmarshal Any message: %w", err)
	}

	t := reflect.TypeOf(msg)
	if p, ok := r.plots[t]; ok {
		p.Add(msg)
	} else {
		log.Printf("Unknown event type: %T", msg)
	}

	return nil
}
