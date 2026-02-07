// Package security provides TLS, identity, and authentication primitives.
//
// TLS files in this package:
//   - tls.go           — Types, self-signed loader, custom cert loader, helpers
//   - tls_selfsigned.go — Self-signed CA + server certificate generation
//   - tls_acme.go       — Let's Encrypt (ACME) automatic certificate management
package security

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/acme/autocert"
)

// TLSConfig holds the paths to the CA and server certificate files.
type TLSConfig struct {
	CACertPath string
	CertPath   string
	KeyPath    string
}

// TLSMode describes how the server should handle TLS.
type TLSMode int

const (
	// TLSModeOff disables TLS entirely (development only).
	TLSModeOff TLSMode = iota
	// TLSModeSelfSigned uses an auto-generated CA and server certificate.
	TLSModeSelfSigned
	// TLSModeACME uses Let's Encrypt automatic certificate management.
	TLSModeACME
	// TLSModeCustom uses user-provided certificate and key files.
	TLSModeCustom
)

// TLSResult holds the outcome of TLS setup, including the config and
// any ACME manager that needs to be wired into the HTTP server.
type TLSResult struct {
	Config      *tls.Config
	Paths       *TLSConfig        // nil for ACME mode
	ACMEManager *autocert.Manager // non-nil only for ACME mode
	Mode        TLSMode
}

// LoadOrGenerateTLS loads existing self-signed TLS certificates from dataDir
// or generates new ones. Returns a *tls.Config configured for TLS 1.3.
func LoadOrGenerateTLS(dataDir string) (*tls.Config, *TLSConfig, error) {
	paths := &TLSConfig{
		CACertPath: filepath.Join(dataDir, "ca.crt"),
		CertPath:   filepath.Join(dataDir, "server.crt"),
		KeyPath:    filepath.Join(dataDir, "server.key"),
	}

	// Generate if any file is missing.
	if !fileExists(paths.CACertPath) || !fileExists(paths.CertPath) || !fileExists(paths.KeyPath) {
		if err := generateCerts(paths); err != nil {
			return nil, nil, fmt.Errorf("generate TLS certs: %w", err)
		}
	}

	cert, err := tls.LoadX509KeyPair(paths.CertPath, paths.KeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load TLS keypair: %w", err)
	}

	caCertPEM, err := os.ReadFile(paths.CACertPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load CA cert: %w", err)
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCertPEM)

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS13,
	}

	return tlsCfg, paths, nil
}

// LoadCustomTLS loads user-provided certificate and key files.
func LoadCustomTLS(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load custom TLS keypair: %w", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// ReadCACert returns the PEM-encoded CA certificate.
func ReadCACert(paths *TLSConfig) ([]byte, error) {
	return os.ReadFile(paths.CACertPath)
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
