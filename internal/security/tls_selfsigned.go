package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"time"
)

// generateCerts creates a self-signed CA and server certificate.
// The server cert includes SANs for localhost, the machine hostname,
// and all local IP addresses for LAN development.
func generateCerts(paths *TLSConfig) error {
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
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
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

	serverKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return err
	}

	dnsNames, ipAddrs := collectSANs()

	serverTemplate := &x509.Certificate{
		SerialNumber: newSerial(),
		Subject: pkix.Name{
			Organization: []string{"Platform"},
			CommonName:   "Platform Server",
		},
		DNSNames:    dnsNames,
		IPAddresses: ipAddrs,
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(2 * 365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return err
	}

	if err := writePEM(paths.CACertPath, "CERTIFICATE", caCertDER); err != nil {
		return err
	}
	if err := writePEM(paths.CertPath, "CERTIFICATE", serverCertDER); err != nil {
		return err
	}

	keyBytes, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return err
	}
	return writePEM(paths.KeyPath, "EC PRIVATE KEY", keyBytes)
}

// collectSANs gathers DNS names and IP addresses for the server certificate.
func collectSANs() ([]string, []net.IP) {
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

	return dnsNames, ipAddrs
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
