package datalogger

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// DataLogger is the interface used for logging data stored in protobuf messages.
type DataLogger interface {
	Log(proto.Message) error
}

// LogWriter writes protobuf messages to a common log.
type LogWriter struct {
	mut       sync.Mutex
	dest      io.Writer
	marshaler proto.MarshalOptions
}

var _ DataLogger = (*LogWriter)(nil)

// NewWriter returns a new LogWriter. dest is the io.Writer that the LogWriter should log to.
// The LogWriter will close the dest in the LogWriter.Close() method if the dest implements io.Closer.
func NewWriter(dest io.Writer) *LogWriter {
	return &LogWriter{
		dest:      dest,
		marshaler: proto.MarshalOptions{},
	}
}

// Close closes the LogWriter. If the dest writer implements io.Closer, it will be closed.
func (w *LogWriter) Close() error {
	if closer, ok := w.dest.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Log logs the proto message.
func (w *LogWriter) Log(msg proto.Message) error {
	any, err := anypb.New(msg)
	if err != nil {
		return fmt.Errorf("failed to create Any message: %w", err)
	}

	buf, err := w.marshaler.Marshal(any)
	if err != nil {
		return fmt.Errorf("failed to marshal Any message: %w", err)
	}

	var msgLen [4]byte
	binary.LittleEndian.PutUint32(msgLen[:], uint32(len(buf)))

	w.mut.Lock()
	defer w.mut.Unlock()

	_, err = w.dest.Write(msgLen[:])
	if err != nil {
		return fmt.Errorf("failed to write message length to log: %w", err)
	}

	_, err = w.dest.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write message to log: %w", err)
	}

	return nil
}

// LogReader reads protobuf messages from a log.
type LogReader struct {
	src         io.Reader
	unmarshaler proto.UnmarshalOptions
}

// NewReader returns a new LogReader. src is the io.Reader that the LogReader should read the log from.
// The LogReader will close the src in the LogReader.Close() method if the src implements io.Closer.
func NewReader(src io.Reader) *LogReader {
	return &LogReader{
		src:         src,
		unmarshaler: proto.UnmarshalOptions{},
	}
}

// Close closes the LogReader. If the src reader implements io.Closer, it will be closed.
func (w *LogReader) Close() error {
	if closer, ok := w.src.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Read reads a protobuf message from the log.
func (w *LogReader) Read() (proto.Message, error) {
	var msgLenBuf [4]byte
	_, err := io.ReadFull(w.src, msgLenBuf[:])
	if err != nil {
		return nil, fmt.Errorf("failed to read message length: %w", err)
	}

	msgLen := binary.LittleEndian.Uint32(msgLenBuf[:])
	if msgLen > 2147483648 { // 2 GiB limit
		return nil, errors.New("message length is greater than 2 GiB")
	}

	buf := make([]byte, msgLen)
	_, err = io.ReadFull(w.src, buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}

	any := anypb.Any{}
	err = w.unmarshaler.Unmarshal(buf, &any)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal to Any message: %w", err)
	}

	msg, err := any.UnmarshalNew()
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal message from Any message: %w", err)
	}

	return msg, nil
}

type nopLogger struct{}

func (nopLogger) Log(proto.Message) error { return nil }

// NopLogger returns a logger that does not log anything.
// This is useful for testing and other situations where data logging is disabled.
func NopLogger() DataLogger {
	return nopLogger{}
}
