package util

import (
	"fmt"
	"io"
	"reflect"
	"unsafe"
)

type toBytes interface {
	ToBytes() []byte
}

type bytes interface {
	Bytes() []byte
}

// WriteAllTo writes all the data to the writer.
func WriteAllTo(writer io.Writer, data ...any) (n int64, err error) {
	for _, d := range data {
		var (
			nn  int64
			nnn int
		)
		switch d := d.(type) {
		case io.WriterTo:
			nn, err = d.WriteTo(writer)
		case string:
			nnn, err = writer.Write(unsafeStringToBytes(d))
		case []byte:
			nnn, err = writer.Write(d)
		case toBytes:
			nnn, err = writer.Write(d.ToBytes())
		case bytes:
			nnn, err = writer.Write(d.Bytes())
		case nil:
		default:
			panic(fmt.Sprintf("cannot write %T", d))
		}
		nn += int64(nnn)
		n += int64(nn)
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func unsafeStringToBytes(s string) []byte {
	if s == "" {
		return []byte{}
	}
	const max = 0x7fff0000
	if len(s) > max {
		panic("string too long")
	}
	return (*[max]byte)(unsafe.Pointer((*reflect.StringHeader)(unsafe.Pointer(&s)).Data))[:len(s):len(s)]
}
