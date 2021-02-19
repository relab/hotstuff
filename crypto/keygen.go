package crypto

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
)

const privateKeyFileType = "HOTSTUFF PRIVATE KEY"
const publicKeyFileType = "HOTSTUFF PUBLIC KEY"

// GeneratePrivateKey returns a new public/private key pair based on ECDSA.
func GeneratePrivateKey() (pk *ecdsa.PrivateKey, err error) {
	curve := elliptic.P256()
	pk, err = ecdsa.GenerateKey(curve, rand.Reader)
	return
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

// WritePrivateKeyFile writes a private key to the specified file.
func WritePrivateKeyFile(key *ecdsa.PrivateKey, filePath string) (err error) {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return
	}

	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	marshalled, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return
	}

	b := &pem.Block{
		Type:  privateKeyFileType,
		Bytes: marshalled,
	}

	err = pem.Encode(f, b)
	return
}

// WritePublicKeyFile writes a public key to the specified file.
func WritePublicKeyFile(key *ecdsa.PublicKey, filePath string) (err error) {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return
	}

	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	marshalled, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return
	}

	b := &pem.Block{
		Type:  publicKeyFileType,
		Bytes: marshalled,
	}

	err = pem.Encode(f, b)
	return
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

func readPemFile(file string) (b *pem.Block, err error) {
	d, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	b, _ = pem.Decode(d)
	if b == nil {
		return nil, fmt.Errorf("failed to decode PEM")
	}
	return b, nil
}

// ReadPrivateKeyFile reads a private key from the specified file.
func ReadPrivateKeyFile(keyFile string) (key *ecdsa.PrivateKey, err error) {
	b, err := readPemFile(keyFile)
	if err != nil {
		return nil, err
	}

	if b.Type != privateKeyFileType {
		return nil, fmt.Errorf("file type did not match")
	}

	key, err = x509.ParseECPrivateKey(b.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse key: %w", err)
	}
	return
}

// ReadPublicKeyFile reads a public key from the specified file.
func ReadPublicKeyFile(keyFile string) (key *ecdsa.PublicKey, err error) {
	b, err := readPemFile(keyFile)
	if err != nil {
		return nil, err
	}

	if b.Type != publicKeyFileType {
		return nil, fmt.Errorf("file type did not match")
	}

	k, err := x509.ParsePKIXPublicKey(b.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse key: %w", err)
	}

	key, ok := k.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key was of wrong type")
	}

	return
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
