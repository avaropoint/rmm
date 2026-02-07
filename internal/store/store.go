// Package store defines the persistence interface for the platform.
// All implementations (SQLite, PostgreSQL, etc.) satisfy the Store interface,
// allowing the server to swap backends without changing business logic.
package store

import (
	"context"
	"time"
)

// Store is the persistence interface for all platform data.
// Implementations must be safe for concurrent use.
type Store interface {
	// Agent management (enrolled agents).
	CreateAgent(ctx context.Context, agent *AgentRecord) error
	GetAgent(ctx context.Context, id string) (*AgentRecord, error)
	GetAgentByCredential(ctx context.Context, credentialHash string) (*AgentRecord, error)
	UpdateAgentSeen(ctx context.Context, id string, t time.Time) error
	ListAgents(ctx context.Context) ([]*AgentRecord, error)
	DeleteAgent(ctx context.Context, id string) error

	// Enrollment tokens.
	CreateEnrollmentToken(ctx context.Context, token *EnrollmentToken) error
	ConsumeEnrollmentToken(ctx context.Context, codeHash string, agentID string) (*EnrollmentToken, error)
	ListEnrollmentTokens(ctx context.Context) ([]*EnrollmentToken, error)
	DeleteEnrollmentToken(ctx context.Context, id string) error

	// API keys.
	CreateAPIKey(ctx context.Context, key *APIKey) error
	VerifyAPIKey(ctx context.Context, keyHash string) (*APIKey, error)
	ListAPIKeys(ctx context.Context) ([]*APIKey, error)
	DeleteAPIKey(ctx context.Context, id string) error

	// Close releases database resources.
	Close() error
}

// AgentRecord is the persistent record for an enrolled agent.
type AgentRecord struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Hostname       string    `json:"hostname"`
	OS             string    `json:"os"`
	Arch           string    `json:"arch"`
	CredentialHash string    `json:"-"`
	EnrolledAt     time.Time `json:"enrolled_at"`
	LastSeen       time.Time `json:"last_seen"`
}

// EnrollmentToken authorises a single agent enrollment.
type EnrollmentToken struct {
	ID        string     `json:"id"`
	CodeHash  string     `json:"-"`
	Type      string     `json:"type"`  // "attended" or "unattended"
	Label     string     `json:"label"` // human-readable description
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	UsedBy    string     `json:"used_by,omitempty"`
}

// APIKey grants access to the management dashboard and APIs.
type APIKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	KeyHash   string     `json:"-"`
	Prefix    string     `json:"prefix"` // first 12 chars for identification
	CreatedAt time.Time  `json:"created_at"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
}
