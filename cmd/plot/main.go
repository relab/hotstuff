package main

import (
	"log"
	"os"

	"github.com/relab/hotstuff/internal/plotting"
	_ "github.com/relab/hotstuff/metrics/types"
)

func main() {
	f, err := os.Open("foo/localhost/measurements.json")
	if err != nil {
		log.Fatal(err)
	}
	p := plotting.NewReader(f)
	err = p.ReadAll()
	if err != nil {
		log.Fatal(err)
	}
}
