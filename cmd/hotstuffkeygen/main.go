package main

import (
	"errors"
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
	info, err := os.Stat(dest)
	if errors.Is(err, os.ErrNotExist) {
		err = os.MkdirAll(dest, 0755)
		if err != nil {
			logger.Fatalf("Cannot create '%s' directory: %v\n", dest, err)
		}
	} else if err != nil {
		logger.Fatalf("Cannot Stat '%s': %v\n", dest, err)
	} else if !info.IsDir() {
		logger.Fatalf("Destination '%s' is not a directory!\n", dest)
	}

	for i := 1; i <= *numKeys; i++ {
		pk, err := data.GeneratePrivateKey()
		if err != nil {
			logger.Fatalf("Failed to generate key: %v\n", err)
		}

		privKeyPath := filepath.Join(dest, strings.ReplaceAll(*keyPattern, "*", fmt.Sprintf("%d", i)))
		pubKeyPath := privKeyPath + ".pub"

		err = data.WritePrivateKeyFile(pk, privKeyPath)
		if err != nil {
			logger.Fatalf("Failed to write private key file: %v\n", err)
		}

		err = data.WritePublicKeyFile(&pk.PublicKey, pubKeyPath)
		if err != nil {
			logger.Fatalf("Failed to write public key file: %v\n", err)
		}
	}
}
