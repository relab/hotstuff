package hotstuff

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
)

const privateKeyFileType = "HOTSTUFF PRIVATE KEY"
const publicKeyFileType = "HOTSTUFF PUBLIC KEY"

// GeneratePrivateKey returns a new public/private key pair based on ECDSA.
func GeneratePrivateKey() (pk *ecdsa.PrivateKey, err error) {
	curve := elliptic.P256()
	pk, err = ecdsa.GenerateKey(curve, rand.Reader)
	return
}

// WritePrivateKeyFile writes a private key to the specified file
func WritePrivateKeyFile(key *ecdsa.PrivateKey, filePath string) error {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	marshalled, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}

	b := &pem.Block{
		Type:  privateKeyFileType,
		Bytes: marshalled,
	}

	pem.Encode(f, b)
	return nil
}

// WritePublicKeyFile writes a public key to the specified file
func WritePublicKeyFile(key *ecdsa.PublicKey, filePath string) error {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	marshalled, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return err
	}

	b := &pem.Block{
		Type:  publicKeyFileType,
		Bytes: marshalled,
	}

	pem.Encode(f, b)
	return nil
}

// ReadPrivateKeyFile reads a private key from the specified file
func ReadPrivateKeyFile(keyFile string) (key *ecdsa.PrivateKey, err error) {
	d, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}

	b, _ := pem.Decode(d)
	if b == nil {
		return nil, fmt.Errorf("Failed to decode key")
	}

	if b.Type != privateKeyFileType {
		return nil, fmt.Errorf("File type did not match")
	}

	key, err = x509.ParseECPrivateKey(b.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse key: %w", err)
	}
	return
}

// ReadPublicKeyFile reads a public key from the specified file
func ReadPublicKeyFile(keyFile string) (key *ecdsa.PublicKey, err error) {
	d, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}

	b, _ := pem.Decode(d)
	if b == nil {
		return nil, fmt.Errorf("Failed to decode key")
	}

	if b.Type != publicKeyFileType {
		return nil, fmt.Errorf("File type did not match")
	}

	k, err := x509.ParsePKIXPublicKey(b.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse key: %w", err)
	}

	key, ok := k.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("Key was of wrong type")
	}

	return
}
