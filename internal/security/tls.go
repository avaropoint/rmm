package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

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
		// TLS 1.3 in Go 1.23+ automatically negotiates X25519+ML-KEM-768
		// hybrid post-quantum key exchange with compatible peers.
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

// NewACMEManager creates a Let's Encrypt autocert manager for the given domains.
// Certificates are cached in dataDir/acme-certs.
func NewACMEManager(dataDir string, domains ...string) (*autocert.Manager, *tls.Config) {
	cacheDir := filepath.Join(dataDir, "acme-certs")
	_ = os.MkdirAll(cacheDir, 0700)

	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(domains...),
		Cache:      autocert.DirCache(cacheDir),
	}

	tlsCfg := manager.TLSConfig()
	tlsCfg.MinVersion = tls.VersionTLS13

	return manager, tlsCfg
}

// ReadCACert returns the PEM-encoded CA certificate.
func ReadCACert(paths *TLSConfig) ([]byte, error) {
	return os.ReadFile(paths.CACertPath)
}

func generateCerts(paths *TLSConfig) error {
	// Generate CA key.
	caKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return err
	}

	caTemplate := &x509.Certificate{
		SerialNumber: newSerial(),
		Subject: pkix.Name{
			Organization: []string{"Platform CA"},
			CommonName:   "Platform Root CA",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return err
	}

	// Generate server key.
	serverKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return err
	}

	// Collect SANs: localhost, hostname, all local IPs.
	dnsNames := []string{"localhost"}
	var ipAddrs []net.IP

	if hostname, err := os.Hostname(); err == nil {
		dnsNames = append(dnsNames, hostname)
	}

	ipAddrs = append(ipAddrs, net.IPv4(127, 0, 0, 1), net.IPv6loopback)

	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && !ip.IsLoopback() {
					ipAddrs = append(ipAddrs, ip)
				}
			}
		}
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: newSerial(),
		Subject: pkix.Name{
			Organization: []string{"Platform"},
			CommonName:   "Platform Server",
		},
		DNSNames:    dnsNames,
		IPAddresses: ipAddrs,
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(2 * 365 * 24 * time.Hour), // 2 years
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return err
	}

	// Write CA cert.
	if err := writePEM(paths.CACertPath, "CERTIFICATE", caCertDER); err != nil {
		return err
	}

	// Write server cert.
	if err := writePEM(paths.CertPath, "CERTIFICATE", serverCertDER); err != nil {
		return err
	}

	// Write server key.
	keyBytes, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return err
	}

	return writePEM(paths.KeyPath, "EC PRIVATE KEY", keyBytes)
}

func writePEM(path, blockType string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	return pem.Encode(f, &pem.Block{Type: blockType, Bytes: data})
}

func newSerial() *big.Int {
	max := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, _ := rand.Int(rand.Reader, max)
	return serial
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
