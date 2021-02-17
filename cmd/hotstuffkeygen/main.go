package main

import (
	"crypto/ecdsa"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/relab/hotstuff"
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

	if *tls && len(*hosts) > 1 && len(*hosts) != *numKeys {
		logger.Fatalf("You must specify one host or IP for each certificate to generate.")
	}

	var caKey *ecdsa.PrivateKey
	var ca *x509.Certificate
	if *tls {
		caKey, ca = createRootCA(dest)
	}

	for i := 0; i < *numKeys; i++ {
		pk, err := crypto.GeneratePrivateKey()
		if err != nil {
			logger.Fatalf("Failed to generate key: %v\n", err)
		}

		basePath := filepath.Join(dest, strings.ReplaceAll(*keyPattern, "*", fmt.Sprintf("%d", *startID+i)))
		certPath := basePath + ".crt"
		privKeyPath := basePath + ".key"
		pubKeyPath := privKeyPath + ".pub"

		if *tls {
			createTLSCert(certPath, i, hotstuff.ID(*startID+i), *hosts, ca, caKey, &pk.PublicKey)
		}

		err = crypto.WritePrivateKeyFile(pk, privKeyPath)
		if err != nil {
			logger.Fatalf("Failed to write private key file: %v\n", err)
		}

		err = crypto.WritePublicKeyFile(&pk.PublicKey, pubKeyPath)
		if err != nil {
			logger.Fatalf("Failed to write public key file: %v\n", err)
		}
	}

}

func createRootCA(dest string) (pk *ecdsa.PrivateKey, ca *x509.Certificate) {
	var err error
	pk, err = crypto.GeneratePrivateKey()
	if err != nil {
		logger.Fatalln("Failed to generate signing key: ", err)
	}
	ca, err = crypto.GenerateRootCert(pk)
	if err != nil {
		logger.Fatalln("Failed to generate root certificate: ", err)
	}
	err = crypto.WriteCertFile(ca, filepath.Join(dest, "ca.crt"))
	if err != nil {
		logger.Fatalln("Failed to write root certificate: ", err)
	}
	return pk, ca
}

func createTLSCert(path string, i int, id hotstuff.ID, hosts []string, ca *x509.Certificate, priv *ecdsa.PrivateKey, pub *ecdsa.PublicKey) {
	var host string
	if len(hosts) == 1 {
		host = hosts[0]
	} else {
		host = hosts[i]
	}
	cert, err := crypto.GenerateTLSCert(id, []string{host}, ca, pub, priv)
	if err != nil {
		logger.Printf("Failed to generate TLS certificate: %v\n", err)
	}
	err = crypto.WriteCertFile(cert, path)
	if err != nil {
		logger.Printf("Failed to write certificate to file: %v\n", err)
	}
}
