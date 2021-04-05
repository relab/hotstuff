package main

import (
	"fmt"
	"log"
	"os"

	"github.com/relab/hotstuff/crypto"
	"github.com/spf13/pflag"
)

const defaultPattern = "*"

var logger = log.New(os.Stderr, "", 0)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] [destination]\n", os.Args[0])
	pflag.PrintDefaults()
}

func main() {
	pflag.Usage = usage
	var (
		startID    = pflag.IntP("start-id", "i", 1, "The ID of the first replica.")
		tls        = pflag.Bool("tls", false, "Generate self-signed TLS certificates. (Must also specify hosts)")
		bls        = pflag.Bool("bls", false, "Generate bls12-381 keys.")
		keyPattern = pflag.StringP("pattern", "p", defaultPattern, "Pattern for key file naming. '*' will be replaced by a number.")
		numKeys    = pflag.IntP("num", "n", 1, "Number of keys to generate")
		hosts      = pflag.StringSliceP("hosts", "h", []string{}, "Comma-separated list of hostnames or IPs. One for each replica. Or you can use one value for all replicas.")
	)
	pflag.Parse()

	if pflag.NArg() < 1 {
		usage()
		os.Exit(1)
	}

	dest := pflag.Arg(0)
	err := crypto.GenerateConfiguration(dest, *tls, *bls, *startID, *numKeys, *keyPattern, *hosts)
	if err != nil {
		logger.Fatal(err)
	}
}
