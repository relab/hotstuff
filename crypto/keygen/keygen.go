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
	"path/filepath"
	"strings"
	"time"

	"github.com/relab/hotstuff"
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

// WritePrivateKeyFile writes a private key to the specified file.
func WritePrivateKeyFile(key hotstuff.PrivateKey, filePath string) (err error) {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return
	}

	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	var marshalled []byte
	var keyType string
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		marshalled, err = x509.MarshalECPrivateKey(k)
		if err != nil {
			return
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

	err = pem.Encode(f, b)
	return
}

// WritePublicKeyFile writes a public key to the specified file.
func WritePublicKeyFile(key hotstuff.PublicKey, filePath string) (err error) {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return
	}

	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	var marshalled []byte
	var keyType string
	switch k := key.(type) {
	case *ecdsa.PublicKey:
		marshalled, err = x509.MarshalPKIXPublicKey(k)
		if err != nil {
			return
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
func ReadPrivateKeyFile(keyFile string) (key hotstuff.PrivateKey, err error) {
	b, err := readPemFile(keyFile)
	if err != nil {
		return nil, err
	}

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

// ReadPublicKeyFile reads a public key from the specified file.
func ReadPublicKeyFile(keyFile string) (key hotstuff.PublicKey, err error) {
	b, err := readPemFile(keyFile)
	if err != nil {
		return nil, err
	}

	switch b.Type {
	case ecdsacrypto.PublicKeyFileType:
		key, err = x509.ParsePKIXPublicKey(b.Bytes)
	case bls12.PublicKeyFileType:
		k := &bls12.PublicKey{}
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

// GenerateConfiguration creates keys and certificates for a configuration of 'n' replicas.
// The keys and certificates are saved in the directory specified by 'dest'.
// 'firstID' specifies the ID of the first replica in the configuration.
// The last ID will be 'firstID' + 'n'.
// 'pattern' describes the pattern for naming of key files.
// For example, '*.key' would result in private keys with the name '1.key', if '1' is the starting ID.
// 'hosts' specify the hosts for which the generated certificates should be valid.
// If len('hosts') is 1, then all certificates will be valid for the same host.
// If not, one of the hosts specified in 'hosts' will be used for each replica.
func GenerateConfiguration(dest string, tls, bls bool, firstID, n int, pattern string, hosts []string) error {
	err := os.MkdirAll(dest, 0755)
	if err != nil {
		return fmt.Errorf("cannot create '%s' directory: %w", dest, err)
	}

	if tls && len(hosts) > 1 && len(hosts) != n {
		return fmt.Errorf("you must specify one host or IP for each certificate to generate")
	}

	var caKey *ecdsa.PrivateKey
	var ca *x509.Certificate
	if tls {
		caKey, ca, err = createRootCA(dest)
		if err != nil {
			return err
		}
	}

	for i := 0; i < n; i++ {
		pk, err := GenerateECDSAPrivateKey()
		if err != nil {
			return fmt.Errorf("failed to generate key: %w", err)
		}

		basePath := filepath.Join(dest, strings.ReplaceAll(pattern, "*", fmt.Sprintf("%d", firstID+i)))
		certPath := basePath + ".crt"
		privKeyPath := basePath + ".key"
		blsPrivPath := basePath + ".bls"

		if tls {
			err = createTLSCert(certPath, i, hotstuff.ID(firstID+i), hosts, ca, caKey, &pk.PublicKey)
			if err != nil {
				return err
			}
		}

		err = writeKeyFiles(pk, privKeyPath)
		if err != nil {
			return err
		}

		if !bls {
			continue
		}

		blsKey, err := bls12.GeneratePrivateKey()
		if err != nil {
			return fmt.Errorf("failed to generate bls12-381 private key: %w", err)
		}

		err = writeKeyFiles(blsKey, blsPrivPath)
		if err != nil {
			return err
		}
	}
	return nil
}

// writeKeyFiles writes both the private and public keys to files.
func writeKeyFiles(key hotstuff.PrivateKey, keyPath string) (err error) {
	err = WritePrivateKeyFile(key, keyPath)
	if err != nil {
		return fmt.Errorf("failed to write private key file: %w", err)
	}
	pubKeyPath := keyPath + ".pub"
	err = WritePublicKeyFile(key.Public(), pubKeyPath)
	if err != nil {
		return fmt.Errorf("failed to write public key file: %w", err)
	}
	return nil
}

func createRootCA(dest string) (pk *ecdsa.PrivateKey, ca *x509.Certificate, err error) {
	pk, err = GenerateECDSAPrivateKey()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate signing key: %w", err)
	}
	ca, err = GenerateRootCert(pk)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate root certificate: %w", err)
	}
	err = WriteCertFile(ca, filepath.Join(dest, "ca.crt"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write root certificate: %w", err)
	}
	return pk, ca, nil
}

func createTLSCert(path string, i int, id hotstuff.ID, hosts []string, ca *x509.Certificate, priv *ecdsa.PrivateKey, pub *ecdsa.PublicKey) error {
	var host string
	if len(hosts) == 1 {
		host = hosts[0]
	} else {
		host = hosts[i]
	}
	cert, err := GenerateTLSCert(id, []string{host}, ca, pub, priv)
	if err != nil {
		return fmt.Errorf("failed to generate TLS certificate: %w", err)
	}
	err = WriteCertFile(cert, path)
	if err != nil {
		return fmt.Errorf("failed to write certificate to file: %w", err)
	}
	return nil
}
