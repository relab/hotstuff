package protostream

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Writer writes protobuf messages to an io.Writer.
type Writer struct {
	mut       sync.Mutex
	dest      io.Writer
	marshaler proto.MarshalOptions
}

// NewWriter returns a new Writer. dest is the io.Writer that the Writer should write to (the stream).
func NewWriter(dest io.Writer) *Writer {
	return &Writer{
		dest:      dest,
		marshaler: proto.MarshalOptions{},
	}
}

// Write writes the proto message to the stream.
func (w *Writer) Write(msg proto.Message) error {
	any, err := anypb.New(msg)
	if err != nil {
		return fmt.Errorf("protostream: failed to create Any message: %w", err)
	}

	buf, err := w.marshaler.Marshal(any)
	if err != nil {
		return fmt.Errorf("protostream: failed to marshal Any message: %w", err)
	}

	var msgLen [4]byte
	binary.LittleEndian.PutUint32(msgLen[:], uint32(len(buf)))

	w.mut.Lock()
	defer w.mut.Unlock()

	_, err = w.dest.Write(msgLen[:])
	if err != nil {
		return fmt.Errorf("protostream: failed to write message length to log: %w", err)
	}

	_, err = w.dest.Write(buf)
	if err != nil {
		return fmt.Errorf("protostream: failed to write message to log: %w", err)
	}

	return nil
}

// Reader reads protobuf messages from an io.Reader.
type Reader struct {
	mut         sync.Mutex
	src         io.Reader
	unmarshaler proto.UnmarshalOptions
}

// NewReader returns a new Reader. src is the io.Reader that the Reader should read the log from.
func NewReader(src io.Reader) *Reader {
	return &Reader{
		src:         src,
		unmarshaler: proto.UnmarshalOptions{},
	}
}

// Read reads a protobuf message from the stream.
func (w *Reader) Read() (proto.Message, error) {
	w.mut.Lock()
	defer w.mut.Unlock()

	var msgLenBuf [4]byte
	_, err := io.ReadFull(w.src, msgLenBuf[:])
	if err != nil {
		return nil, fmt.Errorf("protostream: failed to read message length: %w", err)
	}

	msgLen := binary.LittleEndian.Uint32(msgLenBuf[:])
	if msgLen > 2147483648 { // 2 GiB limit
		return nil, errors.New("protostream: message length is greater than 2 GiB")
	}

	buf := make([]byte, msgLen)
	_, err = io.ReadFull(w.src, buf)
	if err != nil {
		return nil, fmt.Errorf("protostream: failed to read message: %w", err)
	}

	any := anypb.Any{}
	err = w.unmarshaler.Unmarshal(buf, &any)
	if err != nil {
		return nil, fmt.Errorf("protostream: failed to unmarshal to Any message: %w", err)
	}

	msg, err := any.UnmarshalNew()
	if err != nil {
		return nil, fmt.Errorf("protostream: failed to unmarshal message from Any message: %w", err)
	}

	return msg, nil
}
