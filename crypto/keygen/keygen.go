// Package keygen provides helper methods for generating, serializing,
// and deserializing public keys, private keys and certificates.
package keygen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto/bls12"
	ecdsacrypto "github.com/relab/hotstuff/crypto/ecdsa"
)

// GenerateECDSAPrivateKey returns a new ECDSA private key.
func GenerateECDSAPrivateKey() (pk *ecdsa.PrivateKey, err error) {
	pk, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return pk, nil
}

// GenerateRootCert generates a self-signed TLS certificate to act as a CA.
func GenerateRootCert(privateKey *ecdsa.PrivateKey) (cert *x509.Certificate, err error) {
	sn, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	caTmpl := &x509.Certificate{
		SerialNumber:          sn,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificate(caBytes)
}

// GenerateTLSCert generates a TLS certificate for the server that is valid for the given hosts.
func GenerateTLSCert(id hotstuff.ID, hosts []string, parent *x509.Certificate, signeeKey *ecdsa.PublicKey, signerKey *ecdsa.PrivateKey) (cert *x509.Certificate, err error) {
	sn, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	caTmpl := &x509.Certificate{
		SerialNumber: sn,
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("%d", id),
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			caTmpl.IPAddresses = append(caTmpl.IPAddresses, ip)
		} else {
			caTmpl.DNSNames = append(caTmpl.DNSNames, h)
		}
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, caTmpl, parent, signeeKey, signerKey)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificate(caBytes)
}

// PrivateKeyToPEM encodes the private key in PEM format.
func PrivateKeyToPEM(key consensus.PrivateKey) ([]byte, error) {
	var (
		marshalled []byte
		keyType    string
		err        error
	)
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		marshalled, err = x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, err
		}
		keyType = ecdsacrypto.PrivateKeyFileType
	case *bls12.PrivateKey:
		marshalled = k.ToBytes()
		keyType = bls12.PrivateKeyFileType
	}
	b := &pem.Block{
		Type:  keyType,
		Bytes: marshalled,
	}
	return pem.EncodeToMemory(b), nil
}

// WritePrivateKeyFile writes a private key to the specified file.
func WritePrivateKeyFile(key consensus.PrivateKey, filePath string) (err error) {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	b, err := PrivateKeyToPEM(key)
	if err != nil {
		return
	}

	_, err = f.Write(b)
	return
}

// PublicKeyToPEM encodes the public key in PEM format.
func PublicKeyToPEM(key consensus.PublicKey) ([]byte, error) {
	var (
		marshalled []byte
		keyType    string
		err        error
	)
	switch k := key.(type) {
	case *ecdsa.PublicKey:
		marshalled, err = x509.MarshalPKIXPublicKey(k)
		if err != nil {
			return nil, err
		}
		keyType = ecdsacrypto.PublicKeyFileType
	case *bls12.PublicKey:
		marshalled = k.ToBytes()
		keyType = bls12.PublicKeyFileType
	}

	b := &pem.Block{
		Type:  keyType,
		Bytes: marshalled,
	}

	return pem.EncodeToMemory(b), nil
}

// WritePublicKeyFile writes a public key to the specified file.
func WritePublicKeyFile(key consensus.PublicKey, filePath string) (err error) {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return
	}

	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	b, err := PublicKeyToPEM(key)
	if err != nil {
		return err
	}

	_, err = f.Write(b)
	return err
}

// WriteCertFile writes an x509 certificate to a file.
func WriteCertFile(cert *x509.Certificate, file string) (err error) {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return
	}

	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	b := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}

	return pem.Encode(f, b)
}

// ParsePrivateKey parses a PEM encoded private key.
func ParsePrivateKey(buf []byte) (key consensus.PrivateKey, err error) {
	b, _ := pem.Decode(buf)
	switch b.Type {
	case ecdsacrypto.PrivateKeyFileType:
		key, err = x509.ParseECPrivateKey(b.Bytes)
	case bls12.PrivateKeyFileType:
		k := &bls12.PrivateKey{}
		k.FromBytes(b.Bytes)
		key = k
	default:
		return nil, fmt.Errorf("file type did not match any known types")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse key: %w", err)
	}
	return
}

// ReadPrivateKeyFile reads a private key from the specified file.
func ReadPrivateKeyFile(keyFile string) (key consensus.PrivateKey, err error) {
	b, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	return ParsePrivateKey(b)
}

// ParsePublicKey parses a PEM encoded public key
func ParsePublicKey(buf []byte) (key consensus.PublicKey, err error) {
	b, _ := pem.Decode(buf)
	if b == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	switch b.Type {
	case ecdsacrypto.PublicKeyFileType:
		key, err = x509.ParsePKIXPublicKey(b.Bytes)
	case bls12.PublicKeyFileType:
		k := &bls12.PublicKey{}
		err = k.FromBytes(b.Bytes)
		if err != nil {
			return nil, err
		}
		key = k
	default:
		return nil, fmt.Errorf("file type did not match any known types")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse key: %w", err)
	}
	return
}

// ReadPublicKeyFile reads a public key from the specified file.
func ReadPublicKeyFile(keyFile string) (key consensus.PublicKey, err error) {
	b, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	return ParsePublicKey(b)
}

// ReadCertFile read an x509 certificate from a file.
func ReadCertFile(certFile string) (cert *x509.Certificate, err error) {
	d, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}

	b, _ := pem.Decode(d)
	if b == nil {
		return nil, fmt.Errorf("failed to decode key")
	}

	if b.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("file type did not match")
	}

	cert, err = x509.ParseCertificate(b.Bytes)
	if err != nil {
		return nil, err
	}

	return cert, nil
}

// KeyChain contains the keys and certificates needed by a replica, in PEM format.
type KeyChain struct {
	PrivateKey     []byte
	PublicKey      []byte
	Certificate    []byte
	CertificateKey []byte
}

// GenerateKeyChain generates keys and certificates for a replica.
func GenerateKeyChain(id hotstuff.ID, validFor []string, crypto string, ca *x509.Certificate, caKey *ecdsa.PrivateKey) (KeyChain, error) {
	ecdsaKey, err := GenerateECDSAPrivateKey()
	if err != nil {
		return KeyChain{}, err
	}
	certKeyPEM, err := PrivateKeyToPEM(ecdsaKey)
	if err != nil {
		return KeyChain{}, err
	}

	cert, err := GenerateTLSCert(id, validFor, ca, &ecdsaKey.PublicKey, caKey)
	if err != nil {
		return KeyChain{}, err
	}

	certPEM := CertToPEM(cert)

	var privateKey consensus.PrivateKey
	switch crypto {
	case "ecdsa":
		privateKey = ecdsaKey
	case "bls12":
		privateKey, err = bls12.GeneratePrivateKey()
		if err != nil {
			return KeyChain{}, fmt.Errorf("failed to generate bls12-381 private key: %w", err)
		}
	default:
		return KeyChain{}, fmt.Errorf("unknown crypto implementation: %s", crypto)
	}

	privateKeyPEM, err := PrivateKeyToPEM(privateKey)
	if err != nil {
		return KeyChain{}, err
	}

	publicKeyPEM, err := PublicKeyToPEM(privateKey.Public())
	if err != nil {
		return KeyChain{}, err
	}

	return KeyChain{
		PrivateKey:     privateKeyPEM,
		PublicKey:      publicKeyPEM,
		Certificate:    certPEM,
		CertificateKey: certKeyPEM,
	}, nil
}

// GenerateCA returns a certificate authority for generating new certificates.
func GenerateCA() (pk *ecdsa.PrivateKey, ca *x509.Certificate, err error) {
	pk, err = GenerateECDSAPrivateKey()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate signing key: %w", err)
	}
	ca, err = GenerateRootCert(pk)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate root certificate: %w", err)
	}
	return pk, ca, nil
}

// CertToPEM encodes an x509 certificate in PEM format.
func CertToPEM(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
}
