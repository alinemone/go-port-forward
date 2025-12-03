package cert

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"software.sslmate.com/src/go-pkcs12"
)

// P12Config holds P12 certificate configuration
type P12Config struct {
	P12Path      string // Path to .p12 file
	CertPath     string // Output path for certificate (PEM)
	KeyPath      string // Output path for private key (PEM)
	extractedDir string // Internal: directory for extracted files
}

// ExtractP12 extracts certificate and private key from P12 file
// Returns paths to extracted cert and key files
func ExtractP12(p12Path, password string) (*P12Config, error) {
	// Read P12 file
	p12Data, err := os.ReadFile(p12Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read P12 file: %w", err)
	}

	// Decode P12 (supports empty password)
	privateKey, certificate, caCerts, err := pkcs12.DecodeChain(p12Data, password)
	if err != nil {
		return nil, fmt.Errorf("failed to decode P12 (check password): %w", err)
	}

	if certificate == nil {
		return nil, fmt.Errorf("no certificate found in P12 file")
	}

	// Create temporary directory for extracted files
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	certDir := filepath.Join(homeDir, ".pf", "certs")
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cert directory: %w", err)
	}

	// Paths for cert and key
	certPath := filepath.Join(certDir, "client-cert.pem")
	keyPath := filepath.Join(certDir, "client-key.pem")

	// Write certificate chain (leaf + intermediates)
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create cert file: %w", err)
	}
	defer certFile.Close()

	// Write leaf certificate
	if err := pem.Encode(certFile, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certificate.Raw,
	}); err != nil {
		return nil, fmt.Errorf("failed to write certificate: %w", err)
	}

	// Write intermediate certificates (if any)
	for _, caCert := range caCerts {
		if err := pem.Encode(certFile, &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: caCert.Raw,
		}); err != nil {
			return nil, fmt.Errorf("failed to write CA certificate: %w", err)
		}
	}

	// Write private key (unencrypted for kubectl compatibility)
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyFile.Close()

	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	if err := pem.Encode(keyFile, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	}); err != nil {
		return nil, fmt.Errorf("failed to write private key: %w", err)
	}

	return &P12Config{
		P12Path:      p12Path,
		CertPath:     certPath,
		KeyPath:      keyPath,
		extractedDir: certDir,
	}, nil
}

// LoadTLSConfig loads a TLS config from P12 (useful for custom HTTP clients)
func LoadTLSConfig(p12Path, password string) (*tls.Config, error) {
	p12Data, err := os.ReadFile(p12Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read P12 file: %w", err)
	}

	privateKey, certificate, caCerts, err := pkcs12.DecodeChain(p12Data, password)
	if err != nil {
		return nil, fmt.Errorf("failed to decode P12: %w", err)
	}

	// Build certificate chain
	certChain := []tls.Certificate{
		{
			Certificate: [][]byte{certificate.Raw},
			PrivateKey:  privateKey,
		},
	}

	// Add CA certs to chain
	for _, caCert := range caCerts {
		certChain[0].Certificate = append(certChain[0].Certificate, caCert.Raw)
	}

	return &tls.Config{
		Certificates: certChain,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// Cleanup removes extracted certificate files
func (c *P12Config) Cleanup() error {
	if c.extractedDir == "" {
		return nil
	}
	return os.RemoveAll(c.extractedDir)
}
