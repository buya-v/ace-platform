package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

const defaultRSAKeySize = 2048

// LoadRSAKeyPair loads an RSA key pair from PEM-encoded files on disk.
// privatePath is the path to the PKCS#1 or PKCS#8 private key PEM file.
// publicPath is the path to the PKIX public key PEM file.
func LoadRSAKeyPair(privatePath, publicPath string) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privKey, err := loadPrivateKey(privatePath)
	if err != nil {
		return nil, nil, fmt.Errorf("load private key: %w", err)
	}

	pubKey, err := loadPublicKey(publicPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load public key: %w", err)
	}

	return privKey, pubKey, nil
}

// LoadRSAPrivateKeyOnly loads only the RSA private key (public key is derived).
func LoadRSAPrivateKeyOnly(privatePath string) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privKey, err := loadPrivateKey(privatePath)
	if err != nil {
		return nil, nil, fmt.Errorf("load private key: %w", err)
	}
	return privKey, &privKey.PublicKey, nil
}

// GenerateRSAKeyPair generates a new RSA key pair for dev/test use.
func GenerateRSAKeyPair() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, defaultRSAKeySize)
	if err != nil {
		return nil, nil, fmt.Errorf("generate RSA key: %w", err)
	}
	return privKey, &privKey.PublicKey, nil
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", path)
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS#8 key is not RSA")
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type: %s", block.Type)
	}
}

func loadPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", path)
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA")
	}

	return rsaPub, nil
}

// WriteRSAKeyPairPEM writes an RSA key pair to PEM files (for test/dev key generation).
func WriteRSAKeyPairPEM(privKey *rsa.PrivateKey, privatePath, publicPath string) error {
	privBytes := x509.MarshalPKCS1PrivateKey(privKey)
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privBytes,
	})
	if err := os.WriteFile(privatePath, privPEM, 0600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		return fmt.Errorf("marshal public key: %w", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	})
	if err := os.WriteFile(publicPath, pubPEM, 0644); err != nil {
		return fmt.Errorf("write public key: %w", err)
	}

	return nil
}
