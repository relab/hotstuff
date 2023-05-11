package fuzz

// this file includes a couple of extra unnecessary tests
// used for propability tests, profiling and other things

import (
	"fmt"
	"math/rand"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestFrequencyErrorFuzz(t *testing.T) {

	frequency := make(map[string]int, 0)

	f := initFuzz()
	for j := 0; j < 1000; j++ {
		errorInfo := new(ErrorInfo)
		errorInfo.Init()

		iterations := 1

		for i := 0; i < iterations; i++ {
			fuzzMessage := createFuzzMessage(f, nil)
			useFuzzMessage(errorInfo, fuzzMessage, nil)
		}

		for key := range errorInfo.panics {
			frequency[key]++
		}
	}

	sum := 0

	for key, val := range frequency {
		sum += val
		fmt.Println(key)
		fmt.Println(val)
		fmt.Println()
	}

	fmt.Println(sum)

}

func BenchmarkFuzz(b *testing.B) {
	errorInfo := new(ErrorInfo)
	errorInfo.Init()

	f := initFuzz()

	for i := 0; i < b.N; i++ {
		seed := rand.Int63()
		fuzzMessage := createFuzzMessage(f, &seed)
		useFuzzMessage(errorInfo, fuzzMessage, &seed)
	}

	fmt.Println(b.Elapsed())

	//errorInfo.OutputInfo(nil)

	fmt.Println()
}

func TestExperimentalString(t *testing.T) {

	f := initFuzz()
	fuzzMessage := createFuzzMessage(f, nil)

	/*fuzzMessage := hotstuffpb.ECDSASignature{
		Signer: uint32(10),
		R:      []byte{3, 1, 2},
		S:      []byte{5, 4, 3},
	}*/

	var msg proto.Message
	msg = fuzzMessage

	fmt.Println(ProtoToString(msg.ProtoReflect(), 0))
}
