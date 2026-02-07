package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/avaropoint/rmm/internal/store"
)

// Token ambiguity-safe alphabet: uppercase + digits, minus O/0/I/1/L.
const tokenAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// GenerateEnrollmentToken creates an enrollment token with a human-readable code.
// Attended tokens use a short code (XXXX-XXXX) and expire in 15 minutes.
// Unattended tokens use a longer code and expire in 7 days.
func GenerateEnrollmentToken(tokenType, label string) (*store.EnrollmentToken, string, error) {
	var codeLen int
	var expiry time.Duration

	switch tokenType {
	case "attended":
		codeLen = 8
		expiry = 15 * time.Minute
	case "unattended":
		codeLen = 24
		expiry = 7 * 24 * time.Hour
	default:
		return nil, "", fmt.Errorf("invalid token type: %s", tokenType)
	}

	code, err := randomCode(codeLen)
	if err != nil {
		return nil, "", err
	}

	now := time.Now()
	token := &store.EnrollmentToken{
		ID:        randomHex(8),
		CodeHash:  hashCode(code),
		Type:      tokenType,
		Label:     label,
		CreatedAt: now,
		ExpiresAt: now.Add(expiry),
	}

	// Format the code for display.
	display := formatCode(code)

	return token, display, nil
}

// HashEnrollmentCode normalises and hashes an enrollment code for DB lookup.
// Strips dashes and whitespace, uppercases, then SHA-256 hashes.
func HashEnrollmentCode(code string) string {
	cleaned := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	return hashCode(cleaned)
}

// GenerateAPIKey creates a new API key with the format rmm_<random>.
func GenerateAPIKey(name string) (*store.APIKey, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", err
	}

	key := "rmm_" + hex.EncodeToString(raw)
	keyHash := hashCode(key)

	apiKey := &store.APIKey{
		ID:        randomHex(8),
		Name:      name,
		KeyHash:   keyHash,
		Prefix:    key[:12],
		CreatedAt: time.Now(),
	}

	return apiKey, key, nil
}

// HashAPIKey returns the SHA-256 hash of an API key for DB lookup.
func HashAPIKey(key string) string {
	return hashCode(key)
}

func randomCode(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code := make([]byte, length)
	for i := range b {
		code[i] = tokenAlphabet[int(b[i])%len(tokenAlphabet)]
	}
	return string(code), nil
}

// formatCode inserts dashes every 4 characters for readability.
func formatCode(code string) string {
	var parts []string
	for i := 0; i < len(code); i += 4 {
		end := i + 4
		if end > len(code) {
			end = len(code)
		}
		parts = append(parts, code[i:end])
	}
	return strings.Join(parts, "-")
}

func hashCode(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}
