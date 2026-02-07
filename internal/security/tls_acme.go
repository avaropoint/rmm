package security

import (
	"crypto/tls"
	"os"
	"path/filepath"

	"golang.org/x/crypto/acme/autocert"
)

// NewACMEManager creates a Let's Encrypt autocert manager for the given domains.
// Certificates are automatically obtained and renewed. Cached in certsDir/acme/.
//
// Usage:
//
//	manager, tlsCfg := security.NewACMEManager(certsDir, "rmm.example.com")
//	go http.ListenAndServe(":80", manager.HTTPHandler(nil))  // HTTP-01 challenges
//	server := &http.Server{Addr: ":443", TLSConfig: tlsCfg}
//	server.ListenAndServeTLS("", "")
func NewACMEManager(certsDir string, domains ...string) (*autocert.Manager, *tls.Config) {
	cacheDir := filepath.Join(certsDir, "acme")
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
