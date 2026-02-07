// Command agent runs the remote desktop agent that connects to the server,
// captures the screen, and forwards input events.
package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/avaropoint/rmm/internal/version"
)

// reconnectDelay is the pause between connection attempts.
const reconnectDelay = 5 * time.Second

// AgentConfig stores enrollment credentials on disk for persistent sessions.
type AgentConfig struct {
	ServerURL   string `json:"server_url"`
	AgentID     string `json:"agent_id"`
	Credential  string `json:"credential"`
	CACert      string `json:"ca_certificate,omitempty"`
	Fingerprint string `json:"platform_fingerprint,omitempty"`
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "rmm", "agent.json")
}

func loadConfig() (*AgentConfig, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return nil, err
	}
	var cfg AgentConfig
	return &cfg, json.Unmarshal(data, &cfg)
}

func saveConfig(cfg *AgentConfig) error {
	dir := filepath.Dir(configPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0600)
}

// enroll performs the HTTPS enrollment handshake with the server.
func enroll(serverURL, code, name string, insecure bool) (*AgentConfig, error) {
	tlsCfg := &tls.Config{InsecureSkipVerify: insecure} //nolint:gosec
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   30 * time.Second,
	}

	hostname, _ := os.Hostname()
	if name == "" {
		name = hostname
	}

	body, _ := json.Marshal(map[string]string{
		"code":     code,
		"name":     name,
		"hostname": hostname,
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
	})

	base := strings.TrimRight(serverURL, "/")
	if !strings.HasPrefix(base, "http") {
		base = "https://" + base
	}

	resp, err := client.Post(base+"/api/enroll", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("enrollment request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp) //nolint:errcheck
		return nil, fmt.Errorf("enrollment rejected: %s", errResp.Error)
	}

	var result struct {
		AgentID     string `json:"agent_id"`
		Credential  string `json:"credential"`
		Fingerprint string `json:"platform_fingerprint"`
		CACert      string `json:"ca_certificate"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse enrollment response: %w", err)
	}

	wsURL := strings.Replace(base, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)

	return &AgentConfig{
		ServerURL:   wsURL,
		AgentID:     result.AgentID,
		Credential:  result.Credential,
		CACert:      result.CACert,
		Fingerprint: result.Fingerprint,
	}, nil
}

// buildTLSConfig creates a TLS configuration from the agent config.
// Trust is established via the CA certificate received during enrollment
// (self-signed mode) or the system CA store (ACME / custom cert mode).
func buildTLSConfig(cfg *AgentConfig, insecure bool) *tls.Config {
	if !strings.HasPrefix(cfg.ServerURL, "wss://") {
		return nil // plain WS — no TLS needed.
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13} //nolint:gosec

	if cfg.CACert != "" {
		// Self-signed: use the CA cert received at enrollment time.
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM([]byte(cfg.CACert))
		tlsCfg.RootCAs = pool
	} else if insecure {
		// Dev mode: skip verification entirely.
		tlsCfg.InsecureSkipVerify = true //nolint:gosec
	}
	// ACME / custom certs: system CA pool is used automatically.

	return tlsCfg
}

func main() {
	serverURL := flag.String("server", "", "Server URL (e.g. https://server:8443)")
	enrollCode := flag.String("enroll", "", "Enrollment code for initial registration")
	name := flag.String("name", "", "Agent name (defaults to hostname)")
	insecure := flag.Bool("insecure", false, "Skip TLS certificate verification")
	flag.Parse()

	log.Printf("Agent v%s (built %s)", version.Version, version.BuildTime)
	log.Printf("OS: %s, Arch: %s", runtime.GOOS, runtime.GOARCH)

	var cfg *AgentConfig

	if *enrollCode != "" {
		// Enrollment mode.
		if *serverURL == "" {
			log.Fatal("Server URL required for enrollment (-server)")
		}
		log.Printf("Enrolling with server %s...", *serverURL)

		var err error
		cfg, err = enroll(*serverURL, *enrollCode, *name, *insecure)
		if err != nil {
			log.Fatalf("Enrollment failed: %v", err)
		}

		if err := saveConfig(cfg); err != nil {
			log.Fatalf("Failed to save config: %v", err)
		}
		log.Printf("Enrolled successfully (agent ID: %s)", cfg.AgentID)
		log.Printf("Config saved to %s", configPath())
	} else {
		// Reconnection mode — load saved config.
		var err error
		cfg, err = loadConfig()
		if err != nil {
			if *serverURL != "" {
				// Legacy mode: connect without enrollment.
				wsURL := *serverURL
				if !strings.HasPrefix(wsURL, "ws") {
					wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
					wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
				}
				cfg = &AgentConfig{ServerURL: wsURL}
			} else {
				log.Fatal("Not enrolled. Use: agent -server <url> -enroll <code>")
			}
		}
	}

	log.Printf("Server: %s", cfg.ServerURL)

	agent := &Agent{
		serverURL:  cfg.ServerURL,
		name:       *name,
		credential: cfg.Credential,
		tlsConfig:  buildTLSConfig(cfg, *insecure),
	}

	for {
		if err := agent.run(); err != nil {
			log.Printf("Connection error: %v", err)
		}
		log.Printf("Reconnecting in %s...", reconnectDelay)
		time.Sleep(reconnectDelay)
	}
}
