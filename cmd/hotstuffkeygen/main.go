package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/relab/hotstuff/data"
	"github.com/spf13/pflag"
)

const defaultPattern = "r*.key"

var logger = log.New(os.Stderr, "", 0)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] [destination]\n", os.Args[0])
	pflag.PrintDefaults()
}

func main() {
	pflag.Usage = usage
	var (
		keyPattern = pflag.StringP("pattern", "p", defaultPattern, "Pattern for key file naming. '*' will be replaced by a number.")
		numKeys    = pflag.IntP("num", "n", 1, "Number of keys to generate")
	)
	pflag.Parse()

	if pflag.NArg() < 1 {
		usage()
		os.Exit(1)
	}

	dest := pflag.Arg(0)
	for i := 1; i <= *numKeys; i++ {
		pk, err := data.GeneratePrivateKey()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to generate key: %v\n", err)
			os.Exit(1)
		}

		privKeyPath := filepath.Join(dest, strings.ReplaceAll(*keyPattern, "*", fmt.Sprintf("%d", i)))
		pubKeyPath := privKeyPath + ".pub"

		err = data.WritePrivateKeyFile(pk, privKeyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write private key file: %v\n", err)
			os.Exit(1)
		}

		err = data.WritePublicKeyFile(&pk.PublicKey, pubKeyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write public key file: %v\n", err)
			os.Exit(1)
		}
	}
}
