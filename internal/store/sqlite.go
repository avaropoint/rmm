package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver.
)

// migrations is an ordered list of SQL statements applied on startup.
// Each entry is idempotent (IF NOT EXISTS) so re-running is safe.
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS agents (
		id              TEXT PRIMARY KEY,
		name            TEXT NOT NULL,
		hostname        TEXT NOT NULL DEFAULT '',
		os              TEXT NOT NULL DEFAULT '',
		arch            TEXT NOT NULL DEFAULT '',
		credential_hash TEXT UNIQUE NOT NULL,
		enrolled_at     TEXT NOT NULL,
		last_seen       TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS enrollment_tokens (
		id         TEXT PRIMARY KEY,
		code_hash  TEXT UNIQUE NOT NULL,
		type       TEXT NOT NULL DEFAULT 'attended',
		label      TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		used_at    TEXT,
		used_by    TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS api_keys (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		key_hash   TEXT UNIQUE NOT NULL,
		prefix     TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		last_used  TEXT
	)`,
}

// SQLiteStore implements Store using a SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at path and runs migrations.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	dsn := fmt.Sprintf("%s?_journal=WAL&_busy_timeout=5000", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite handles one writer at a time.

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close() //nolint:errcheck
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	for _, stmt := range migrations {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

// --- Agents ---

func (s *SQLiteStore) CreateAgent(ctx context.Context, a *AgentRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agents (id, name, hostname, os, arch, credential_hash, enrolled_at, last_seen)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Hostname, a.OS, a.Arch,
		a.CredentialHash, a.EnrolledAt.UTC().Format(time.RFC3339), a.LastSeen.UTC().Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) GetAgent(ctx context.Context, id string) (*AgentRecord, error) {
	return s.scanAgent(s.db.QueryRowContext(ctx,
		`SELECT id, name, hostname, os, arch, credential_hash, enrolled_at, last_seen FROM agents WHERE id = ?`, id))
}

func (s *SQLiteStore) GetAgentByCredential(ctx context.Context, credentialHash string) (*AgentRecord, error) {
	return s.scanAgent(s.db.QueryRowContext(ctx,
		`SELECT id, name, hostname, os, arch, credential_hash, enrolled_at, last_seen FROM agents WHERE credential_hash = ?`, credentialHash))
}

func (s *SQLiteStore) UpdateAgentSeen(ctx context.Context, id string, t time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET last_seen = ? WHERE id = ?`, t.UTC().Format(time.RFC3339), id)
	return err
}

func (s *SQLiteStore) ListAgents(ctx context.Context) ([]*AgentRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, hostname, os, arch, credential_hash, enrolled_at, last_seen FROM agents ORDER BY enrolled_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var agents []*AgentRecord
	for rows.Next() {
		a, err := s.scanAgentRows(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *SQLiteStore) DeleteAgent(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) scanAgent(row *sql.Row) (*AgentRecord, error) {
	var a AgentRecord
	var enrolled, seen string
	if err := row.Scan(&a.ID, &a.Name, &a.Hostname, &a.OS, &a.Arch, &a.CredentialHash, &enrolled, &seen); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	a.EnrolledAt, _ = time.Parse(time.RFC3339, enrolled)
	a.LastSeen, _ = time.Parse(time.RFC3339, seen)
	return &a, nil
}

func (s *SQLiteStore) scanAgentRows(rows *sql.Rows) (*AgentRecord, error) {
	var a AgentRecord
	var enrolled, seen string
	if err := rows.Scan(&a.ID, &a.Name, &a.Hostname, &a.OS, &a.Arch, &a.CredentialHash, &enrolled, &seen); err != nil {
		return nil, err
	}
	a.EnrolledAt, _ = time.Parse(time.RFC3339, enrolled)
	a.LastSeen, _ = time.Parse(time.RFC3339, seen)
	return &a, nil
}

// --- Enrollment Tokens ---

func (s *SQLiteStore) CreateEnrollmentToken(ctx context.Context, t *EnrollmentToken) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO enrollment_tokens (id, code_hash, type, label, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.ID, t.CodeHash, t.Type, t.Label,
		t.CreatedAt.UTC().Format(time.RFC3339), t.ExpiresAt.UTC().Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) ConsumeEnrollmentToken(ctx context.Context, codeHash string, agentID string) (*EnrollmentToken, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var t EnrollmentToken
	var created, expires string
	var usedAt, usedBy sql.NullString

	err = tx.QueryRowContext(ctx,
		`SELECT id, code_hash, type, label, created_at, expires_at, used_at, used_by
		 FROM enrollment_tokens WHERE code_hash = ?`, codeHash).
		Scan(&t.ID, &t.CodeHash, &t.Type, &t.Label, &created, &expires, &usedAt, &usedBy)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	t.CreatedAt, _ = time.Parse(time.RFC3339, created)
	t.ExpiresAt, _ = time.Parse(time.RFC3339, expires)

	// Check if already used.
	if usedAt.Valid {
		return nil, fmt.Errorf("enrollment token already used")
	}

	// Check if expired.
	if time.Now().After(t.ExpiresAt) {
		return nil, fmt.Errorf("enrollment token expired")
	}

	// Mark as consumed.
	if _, err := tx.ExecContext(ctx,
		`UPDATE enrollment_tokens SET used_at = ?, used_by = ? WHERE id = ?`,
		now, agentID, t.ID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &t, nil
}

func (s *SQLiteStore) ListEnrollmentTokens(ctx context.Context) ([]*EnrollmentToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, code_hash, type, label, created_at, expires_at, used_at, used_by
		 FROM enrollment_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var tokens []*EnrollmentToken
	for rows.Next() {
		var t EnrollmentToken
		var created, expires string
		var usedAt, usedBy sql.NullString
		if err := rows.Scan(&t.ID, &t.CodeHash, &t.Type, &t.Label, &created, &expires, &usedAt, &usedBy); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, created)
		t.ExpiresAt, _ = time.Parse(time.RFC3339, expires)
		if usedAt.Valid {
			parsed, _ := time.Parse(time.RFC3339, usedAt.String)
			t.UsedAt = &parsed
		}
		t.UsedBy = usedBy.String
		tokens = append(tokens, &t)
	}
	return tokens, rows.Err()
}

func (s *SQLiteStore) DeleteEnrollmentToken(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM enrollment_tokens WHERE id = ?`, id)
	return err
}

// --- API Keys ---

func (s *SQLiteStore) CreateAPIKey(ctx context.Context, k *APIKey) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_keys (id, name, key_hash, prefix, created_at) VALUES (?, ?, ?, ?, ?)`,
		k.ID, k.Name, k.KeyHash, k.Prefix, k.CreatedAt.UTC().Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) VerifyAPIKey(ctx context.Context, keyHash string) (*APIKey, error) {
	var k APIKey
	var created string
	var lastUsed sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, key_hash, prefix, created_at, last_used FROM api_keys WHERE key_hash = ?`, keyHash).
		Scan(&k.ID, &k.Name, &k.KeyHash, &k.Prefix, &created, &lastUsed)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	k.CreatedAt, _ = time.Parse(time.RFC3339, created)

	// Update last_used timestamp.
	now := time.Now()
	k.LastUsed = &now
	_, _ = s.db.ExecContext(ctx,
		`UPDATE api_keys SET last_used = ? WHERE id = ?`,
		now.UTC().Format(time.RFC3339), k.ID)

	return &k, nil
}

func (s *SQLiteStore) ListAPIKeys(ctx context.Context) ([]*APIKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, key_hash, prefix, created_at, last_used FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var keys []*APIKey
	for rows.Next() {
		var k APIKey
		var created string
		var lastUsed sql.NullString
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyHash, &k.Prefix, &created, &lastUsed); err != nil {
			return nil, err
		}
		k.CreatedAt, _ = time.Parse(time.RFC3339, created)
		if lastUsed.Valid {
			parsed, _ := time.Parse(time.RFC3339, lastUsed.String)
			k.LastUsed = &parsed
		}
		keys = append(keys, &k)
	}
	return keys, rows.Err()
}

func (s *SQLiteStore) DeleteAPIKey(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	return err
}
