package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/avaropoint/rmm/internal/security"
	"github.com/avaropoint/rmm/internal/store"
)

// handleListAgents returns a JSON list of all connected agents.
func (s *Server) handleListAgents(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	s.mu.RLock()
	agents := make([]LiveAgent, 0, len(s.agents))
	for _, a := range s.agents {
		agents = append(agents, LiveAgent{
			ID:            a.ID,
			Name:          a.Name,
			Hostname:      a.Hostname,
			OS:            a.OS,
			OSVersion:     a.OSVersion,
			Arch:          a.Arch,
			IP:            a.IP,
			Status:        a.Status,
			LastSeen:      a.LastSeen,
			CPUCount:      a.CPUCount,
			MemoryTotal:   a.MemoryTotal,
			MemoryFree:    a.MemoryFree,
			DiskTotal:     a.DiskTotal,
			DiskFree:      a.DiskFree,
			Displays:      a.Displays,
			DisplayCount:  a.DisplayCount,
			LocalIPs:      a.LocalIPs,
			Username:      a.Username,
			UptimeSeconds: a.UptimeSeconds,
			AgentVersion:  a.AgentVersion,
			EnrolledAt:    a.EnrolledAt,
		})
	}
	s.mu.RUnlock()

	json.NewEncoder(w).Encode(agents) //nolint:errcheck
}

// handleEnroll processes agent enrollment requests.
// Agents POST with an enrollment code and receive credentials in return.
func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Code     string `json:"code"`
		Name     string `json:"name"`
		Hostname string `json:"hostname"`
		OS       string `json:"os"`
		Arch     string `json:"arch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Code == "" {
		http.Error(w, `{"error":"enrollment code required"}`, http.StatusBadRequest)
		return
	}

	codeHash := security.HashEnrollmentCode(req.Code)
	agentID := security.HashAPIKey(req.Code + s.platform.Fingerprint())[:16]

	token, err := s.store.ConsumeEnrollmentToken(context.Background(), codeHash, agentID)
	if err != nil {
		log.Printf("Enrollment failed: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusForbidden)
		return
	}
	if token == nil {
		http.Error(w, `{"error":"invalid enrollment code"}`, http.StatusForbidden)
		return
	}

	credential := s.platform.SignCredential(agentID)
	credHash := security.CredentialHash(credential)

	now := time.Now()
	agentRec := &store.AgentRecord{
		ID:             agentID,
		Name:           req.Name,
		Hostname:       req.Hostname,
		OS:             req.OS,
		Arch:           req.Arch,
		CredentialHash: credHash,
		EnrolledAt:     now,
		LastSeen:       now,
	}
	if err := s.store.CreateAgent(context.Background(), agentRec); err != nil {
		log.Printf("Failed to store agent: %v", err)
		http.Error(w, `{"error":"enrollment failed"}`, http.StatusInternalServerError)
		return
	}

	log.Printf("Agent enrolled: %s (%s) via %s token", req.Name, agentID, token.Type)

	var caCert string
	if s.tlsPaths != nil {
		if data, err := security.ReadCACert(s.tlsPaths); err == nil {
			caCert = string(data)
		}
	}

	resp := map[string]string{
		"agent_id":             agentID,
		"credential":           credential,
		"platform_fingerprint": s.platform.Fingerprint(),
	}
	if caCert != "" {
		resp["ca_certificate"] = caCert
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// handleEnrollmentTokens manages enrollment tokens (CRUD).
func (s *Server) handleEnrollmentTokens(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		tokens, err := s.store.ListEnrollmentTokens(context.Background())
		if err != nil {
			http.Error(w, `{"error":"failed to list tokens"}`, http.StatusInternalServerError)
			return
		}
		if tokens == nil {
			tokens = []*store.EnrollmentToken{}
		}
		json.NewEncoder(w).Encode(tokens) //nolint:errcheck

	case http.MethodPost:
		var req struct {
			Type  string `json:"type"`
			Label string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		if req.Type == "" {
			req.Type = "attended"
		}

		token, code, err := security.GenerateEnrollmentToken(req.Type, req.Label)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}
		if err := s.store.CreateEnrollmentToken(context.Background(), token); err != nil {
			http.Error(w, `{"error":"failed to create token"}`, http.StatusInternalServerError)
			return
		}

		log.Printf("Enrollment token created: %s (%s)", token.ID, req.Type)
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"id":         token.ID,
			"code":       code,
			"type":       token.Type,
			"label":      token.Label,
			"expires_at": token.ExpiresAt,
		})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
			return
		}
		if err := s.store.DeleteEnrollmentToken(context.Background(), id); err != nil {
			http.Error(w, `{"error":"failed to delete"}`, http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}) //nolint:errcheck

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAuthVerify validates an API key.
func (s *Server) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		http.Error(w, `{"error":"key required"}`, http.StatusBadRequest)
		return
	}

	keyHash := security.HashAPIKey(req.Key)
	apiKey, err := s.store.VerifyAPIKey(context.Background(), keyHash)
	if err != nil || apiKey == nil {
		http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
		"valid":    true,
		"name":     apiKey.Name,
		"platform": s.platform.Fingerprint(),
	})
}
