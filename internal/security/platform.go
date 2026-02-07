package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/hkdf"
)

// Platform holds the server's Ed25519 identity keypair and a derived
// symmetric key used for HMAC-SHA-512 credential signing.
type Platform struct {
	PublicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
	credKey    []byte // HKDF-derived key for HMAC credential signing
}

// Fingerprint returns the SHA-256 hex fingerprint of the platform public key.
// This uniquely identifies the deployment instance.
func (p *Platform) Fingerprint() string {
	h := sha256.Sum256(p.PublicKey)
	return hex.EncodeToString(h[:])
}

// SignCredential produces a versioned agent credential:
//
//	v1.<agentID>.<hmac_sha512_hex>
//
// HMAC-SHA-512 is quantum-safe for authentication. The v1 prefix allows
// future upgrades to ML-DSA (FIPS 204) post-quantum signatures.
func (p *Platform) SignCredential(agentID string) string {
	mac := hmacSHA512(p.credKey, []byte("agent-credential:"+agentID))
	return fmt.Sprintf("v1.%s.%s", agentID, hex.EncodeToString(mac))
}

// VerifyCredential checks a v1-format credential string.
// Returns the embedded agent ID on success, or an error.
func (p *Platform) VerifyCredential(credential string) (string, error) {
	// Parse "v1.<agentID>.<hex_mac>"
	if len(credential) < 5 || credential[:3] != "v1." {
		return "", fmt.Errorf("unsupported credential version")
	}

	// Find the last dot to split agentID from MAC.
	lastDot := -1
	for i := len(credential) - 1; i >= 3; i-- {
		if credential[i] == '.' {
			lastDot = i
			break
		}
	}
	if lastDot <= 3 {
		return "", fmt.Errorf("malformed credential")
	}

	agentID := credential[3:lastDot]
	macHex := credential[lastDot+1:]

	providedMAC, err := hex.DecodeString(macHex)
	if err != nil {
		return "", fmt.Errorf("malformed credential MAC")
	}

	expectedMAC := hmacSHA512(p.credKey, []byte("agent-credential:"+agentID))

	if !hmacEqual(providedMAC, expectedMAC) {
		return "", fmt.Errorf("invalid credential")
	}

	return agentID, nil
}

// CredentialHash returns the SHA-256 hash of a credential string,
// used for database lookups without storing the raw credential.
func CredentialHash(credential string) string {
	h := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(h[:])
}

// LoadOrCreatePlatform loads the platform keypair from dataDir or generates one.
func LoadOrCreatePlatform(dataDir string) (*Platform, error) {
	keyPath := filepath.Join(dataDir, "platform.key")
	if fileExists(keyPath) {
		return loadPlatformKey(keyPath)
	}
	return generatePlatformKey(keyPath)
}

func loadPlatformKey(path string) (*Platform, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("invalid platform key file")
	}

	if len(block.Bytes) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid platform key size")
	}

	priv := ed25519.NewKeyFromSeed(block.Bytes)
	return newPlatform(priv), nil
}

func generatePlatformKey(path string) (*Platform, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	seed := priv.Seed()
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: seed}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}

	if err := pem.Encode(f, block); err != nil {
		f.Close() //nolint:errcheck
		return nil, err
	}
	_ = f.Close()

	return newPlatform(priv), nil
}

func newPlatform(priv ed25519.PrivateKey) *Platform {
	// Derive a separate symmetric key for HMAC credential signing.
	// HKDF-SHA-512: deterministic, one-way, quantum-safe key derivation.
	credKey := make([]byte, 64)
	r := hkdf.New(sha512.New, priv.Seed(), []byte("rmm-credential-v1"), []byte("agent-authentication"))
	io.ReadFull(r, credKey) //nolint:errcheck

	return &Platform{
		PublicKey:  priv.Public().(ed25519.PublicKey),
		privateKey: priv,
		credKey:    credKey,
	}
}
