package plotting

import (
	"encoding/json"
	"fmt"
	"io"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/anypb"
)

// Plotter processes measurements from a reader.
type Plotter interface {
	// Adds a measurement to the plotter.
	Add(interface{})
}

// Reader reads measurements from JSON.
type Reader struct {
	plotters []Plotter
	rd       io.Reader
}

// NewReader returns a new reader that reads from the specified source and adds measurements to the plotters.
func NewReader(rd io.Reader, plotters ...Plotter) *Reader {
	return &Reader{
		plotters: plotters,
		rd:       rd,
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

	for _, p := range r.plotters {
		p.Add(msg)
	}

	return nil
}
