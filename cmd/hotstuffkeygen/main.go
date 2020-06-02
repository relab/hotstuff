package main

import (
	"fmt"
	"os"

	"github.com/relab/hotstuff/data"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s [key path]\n", os.Args[0])
		os.Exit(1)
	}

	privKeyPath := os.Args[1]
	pubKeyPath := privKeyPath + ".pub"

	pk, err := data.GeneratePrivateKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate key: %v\n", err)
		os.Exit(1)
	}

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
