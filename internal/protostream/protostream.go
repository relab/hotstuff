// Package protostream implements reading and writing of protobuf messages to data streams.
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

// WriteAny writes a proto message to the stream wrapped in an anypb.Any message.
func (w *Writer) WriteAny(msg proto.Message) error {
	any, err := anypb.New(msg)
	if err != nil {
		return fmt.Errorf("protostream: failed to create Any message: %w", err)
	}
	return w.Write(any)
}

// Write writes a proto message to the stream.
func (w *Writer) Write(msg proto.Message) error {
	buf, err := w.marshaler.Marshal(msg)
	if err != nil {
		return fmt.Errorf("protostream: failed to marshal message: %w", err)
	}

	var msgLen [4]byte
	binary.LittleEndian.PutUint32(msgLen[:], uint32(len(buf)))

	w.mut.Lock()
	defer w.mut.Unlock()

	_, err = w.dest.Write(msgLen[:])
	if err != nil {
		return fmt.Errorf("protostream: failed to write message length: %w", err)
	}

	_, err = w.dest.Write(buf)
	if err != nil {
		return fmt.Errorf("protostream: failed to write message: %w", err)
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

// ReadAny reads a protobuf message wrapped in an anypb.Any message from the stream.
func (r *Reader) ReadAny() (proto.Message, error) {
	any := anypb.Any{}
	err := r.Read(&any)
	if err != nil {
		return nil, err
	}

	msg, err := any.UnmarshalNew()
	if err != nil {
		return nil, fmt.Errorf("protostream: failed to unmarshal message from Any message: %w", err)
	}

	return msg, nil
}

// Read reads a protobuf message from the stream and unmarshals it into the dst message.
func (r *Reader) Read(dst proto.Message) error {
	r.mut.Lock()
	defer r.mut.Unlock()

	var msgLenBuf [4]byte
	_, err := io.ReadFull(r.src, msgLenBuf[:])
	if err != nil {
		return fmt.Errorf("protostream: failed to read message length: %w", err)
	}

	msgLen := binary.LittleEndian.Uint32(msgLenBuf[:])
	if msgLen > 2147483648 { // 2 GiB limit
		return errors.New("protostream: message length is greater than 2 GiB")
	}

	buf := make([]byte, msgLen)
	_, err = io.ReadFull(r.src, buf)
	if err != nil {
		return fmt.Errorf("protostream: failed to read message: %w", err)
	}

	err = r.unmarshaler.Unmarshal(buf, dst)
	if err != nil {
		return fmt.Errorf("protostream: failed to unmarshal message: %w", err)
	}

	return nil
}
