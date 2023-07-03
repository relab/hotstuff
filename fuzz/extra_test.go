package fuzz

// this file includes a couple of extra unnecessary tests
// used for probability tests, profiling and other things

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
)

func TestFrequencyErrorFuzz(t *testing.T) {
	if os.Getenv("FUZZ") == "" {
		t.Skip("Skipping slow test; run with FUZZ=1 to enable")
	}
	frequency := make(map[string]int, 0)

	f := initFuzz()
	for j := 0; j < 1000; j++ {
		errInfo := new(errorInfo)
		errInfo.init()

		iterations := 1
		for i := 0; i < iterations; i++ {
			fuzzMessage := createFuzzMessage(f, nil)
			useFuzzMessage(errInfo, fuzzMessage, nil)
		}

		for key := range errInfo.panics {
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
	errInfo := new(errorInfo)
	errInfo.init()

	f := initFuzz()

	for i := 0; i < b.N; i++ {
		seed := rand.Int63()
		fuzzMessage := createFuzzMessage(f, &seed)
		useFuzzMessage(errInfo, fuzzMessage, &seed)
	}

	fmt.Println(b.Elapsed())
	fmt.Println()
}
