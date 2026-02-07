package main

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/avaropoint/rmm/internal/protocol"
	"github.com/avaropoint/rmm/internal/security"
	"github.com/avaropoint/rmm/internal/store"
)

// registrationTimeout is how long the server waits for the agent's
// initial registration message after the WebSocket handshake.
const registrationTimeout = 30 * time.Second

// LiveAgent represents an active agent connection (in-memory).
type LiveAgent struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Hostname      string                 `json:"hostname"`
	OS            string                 `json:"os"`
	OSVersion     string                 `json:"os_version"`
	Arch          string                 `json:"arch"`
	IP            string                 `json:"ip"`
	Status        string                 `json:"status"`
	LastSeen      time.Time              `json:"last_seen"`
	CPUCount      int                    `json:"cpu_count"`
	MemoryTotal   uint64                 `json:"memory_total"`
	MemoryFree    uint64                 `json:"memory_free"`
	DiskTotal     uint64                 `json:"disk_total"`
	DiskFree      uint64                 `json:"disk_free"`
	Displays      []protocol.DisplayInfo `json:"displays"`
	DisplayCount  int                    `json:"display_count"`
	LocalIPs      []string               `json:"local_ips"`
	Username      string                 `json:"username"`
	UptimeSeconds int64                  `json:"uptime_seconds"`
	AgentVersion  string                 `json:"agent_version"`
	EnrolledAt    time.Time              `json:"enrolled_at,omitempty"`
	conn          net.Conn
	mu            sync.Mutex
}

// Server manages agents, viewers, and platform state.
type Server struct {
	agents   map[string]*LiveAgent
	viewers  map[string]net.Conn
	mu       sync.RWMutex
	webDir   string
	store    store.Store
	platform *security.Platform
	tlsPaths *security.TLSConfig
}

// NewServer creates a new Server instance.
func NewServer(webDir string, db store.Store, platform *security.Platform, tlsPaths *security.TLSConfig) *Server {
	return &Server{
		agents:   make(map[string]*LiveAgent),
		viewers:  make(map[string]net.Conn),
		webDir:   webDir,
		store:    db,
		platform: platform,
		tlsPaths: tlsPaths,
	}
}

// upgradeWebSocket performs the HTTP → WebSocket handshake per RFC 6455.
func upgradeWebSocket(w http.ResponseWriter, r *http.Request) (net.Conn, error) {
	if r.Header.Get("Upgrade") != "websocket" {
		return nil, fmt.Errorf("not a websocket request")
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, fmt.Errorf("missing Sec-WebSocket-Key")
	}

	h := sha1.New()
	h.Write([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	acceptKey := base64.StdEncoding.EncodeToString(h.Sum(nil))

	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("hijacking not supported")
	}

	conn, _, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n"

	if _, err := conn.Write([]byte(response)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return conn, nil
}

// handleAgent manages the lifecycle of an agent connection.
// Agents must present a valid credential in their registration message.
func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	conn, err := upgradeWebSocket(w, r)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		http.Error(w, "WebSocket upgrade failed", http.StatusBadRequest)
		return
	}

	reader := bufio.NewReader(conn)

	// Read registration message.
	_ = conn.SetReadDeadline(time.Now().Add(registrationTimeout))
	opcode, data, err := protocol.ReadFrame(reader)
	if err != nil || opcode != protocol.OpText {
		_ = conn.Close()
		return
	}

	var msg protocol.Message
	if err := json.Unmarshal(data, &msg); err != nil || msg.Type != "register" {
		_ = conn.Close()
		return
	}

	var reg protocol.Registration
	if err := json.Unmarshal(msg.Payload, &reg); err != nil {
		_ = conn.Close()
		return
	}

	// Verify agent credential.
	if reg.Credential == "" {
		log.Printf("Agent rejected: no credential provided")
		_ = conn.Close()
		return
	}

	agentID, err := s.platform.VerifyCredential(reg.Credential)
	if err != nil {
		log.Printf("Agent rejected: invalid credential: %v", err)
		_ = conn.Close()
		return
	}

	// Confirm agent exists in enrollment database.
	credHash := security.CredentialHash(reg.Credential)
	enrolled, err := s.store.GetAgentByCredential(context.Background(), credHash)
	if err != nil || enrolled == nil {
		log.Printf("Agent rejected: not enrolled (id=%s)", agentID)
		_ = conn.Close()
		return
	}

	displayCount := reg.DisplayCount
	if displayCount < 1 {
		displayCount = 1
	}

	agent := &LiveAgent{
		ID:            enrolled.ID,
		Name:          reg.Name,
		Hostname:      reg.Hostname,
		OS:            reg.OS,
		OSVersion:     reg.OSVersion,
		Arch:          reg.Arch,
		IP:            r.RemoteAddr,
		Status:        "online",
		LastSeen:      time.Now(),
		CPUCount:      reg.CPUCount,
		MemoryTotal:   reg.MemoryTotal,
		MemoryFree:    reg.MemoryFree,
		DiskTotal:     reg.DiskTotal,
		DiskFree:      reg.DiskFree,
		Displays:      reg.Displays,
		DisplayCount:  displayCount,
		LocalIPs:      reg.LocalIPs,
		Username:      reg.Username,
		UptimeSeconds: reg.UptimeSeconds,
		AgentVersion:  reg.AgentVersion,
		EnrolledAt:    enrolled.EnrolledAt,
		conn:          conn,
	}

	s.mu.Lock()
	s.agents[agent.ID] = agent
	s.mu.Unlock()

	log.Printf("Agent registered: %s (%s) - %s/%s", agent.Name, agent.ID, agent.OS, agent.Arch)

	respPayload, _ := json.Marshal(map[string]string{"id": enrolled.ID})
	resp, _ := json.Marshal(protocol.Message{
		Type:    "registered",
		Payload: respPayload,
	})
	_ = protocol.WriteServerFrame(conn, protocol.OpText, resp)
	_ = conn.SetReadDeadline(time.Time{})

	defer func() {
		s.mu.Lock()
		delete(s.agents, agent.ID)
		s.mu.Unlock()
		_ = conn.Close()
		_ = s.store.UpdateAgentSeen(context.Background(), agent.ID, time.Now())
		log.Printf("Agent disconnected: %s", agent.Name)
	}()

	// Agent message loop.
	for {
		opcode, data, err := protocol.ReadFrame(reader)
		if err != nil {
			break
		}

		agent.LastSeen = time.Now()

		switch opcode {
		case protocol.OpClose:
			return
		case protocol.OpPing:
			_ = protocol.WriteServerFrame(conn, protocol.OpPong, data)
			continue
		case protocol.OpBinary:
			// Relay binary frames (screen data) to viewer as-is — zero parsing.
			s.mu.RLock()
			if vc, ok := s.viewers[agent.ID]; ok {
				_ = protocol.WriteServerFrame(vc, protocol.OpBinary, data)
			}
			s.mu.RUnlock()
		case protocol.OpText:
			var m protocol.Message
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}

			switch m.Type {
			case "display_switched":
				s.mu.RLock()
				if vc, ok := s.viewers[agent.ID]; ok {
					_ = protocol.WriteServerFrame(vc, protocol.OpText, data)
				}
				s.mu.RUnlock()
			case "heartbeat":
				agent.Status = "online"
			}
		}
	}
}

// handleViewer manages the lifecycle of a viewer connection.
// Requires valid API key via "token" query parameter.
func (s *Server) handleViewer(w http.ResponseWriter, r *http.Request) {
	// Authenticate viewer.
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	keyHash := security.HashAPIKey(token)
	apiKey, err := s.store.VerifyAPIKey(context.Background(), keyHash)
	if err != nil || apiKey == nil {
		http.Error(w, "invalid API key", http.StatusUnauthorized)
		return
	}

	agentID := r.URL.Query().Get("agent")
	if agentID == "" {
		http.Error(w, "agent parameter required", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	agent, exists := s.agents[agentID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	conn, err := upgradeWebSocket(w, r)
	if err != nil {
		log.Printf("Viewer upgrade error: %v", err)
		return
	}

	reader := bufio.NewReader(conn)

	s.mu.Lock()
	s.viewers[agentID] = conn
	s.mu.Unlock()

	log.Printf("Viewer connected to agent: %s", agent.Name)

	agent.mu.Lock()
	startMsg, _ := json.Marshal(protocol.Message{Type: "start_capture"})
	_ = protocol.WriteServerFrame(agent.conn, protocol.OpText, startMsg)
	agent.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.viewers, agentID)
		s.mu.Unlock()

		agent.mu.Lock()
		stopMsg, _ := json.Marshal(protocol.Message{Type: "stop_capture"})
		_ = protocol.WriteServerFrame(agent.conn, protocol.OpText, stopMsg)
		agent.mu.Unlock()

		_ = conn.Close()
		log.Printf("Viewer disconnected from agent: %s", agent.Name)
	}()

	log.Printf("Starting viewer input loop for agent: %s", agent.Name)

	for {
		opcode, data, err := protocol.ReadFrame(reader)
		if err != nil || opcode == protocol.OpClose {
			log.Printf("Viewer read loop ended: opcode=%d, err=%v", opcode, err)
			break
		}

		if opcode == protocol.OpText {
			var m protocol.Message
			if err := json.Unmarshal(data, &m); err != nil {
				log.Printf("Failed to unmarshal viewer message: %v", err)
				continue
			}
			log.Printf("Viewer message type: %s", m.Type)

			if m.Type == "input" || m.Type == "switch_display" {
				log.Printf("Forwarding %s to agent %s", m.Type, agent.Name)
				agent.mu.Lock()
				_ = protocol.WriteServerFrame(agent.conn, protocol.OpText, data)
				agent.mu.Unlock()
			}
		}
	}
}

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

	// Verify enrollment token.
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

	// Generate agent credential.
	credential := s.platform.SignCredential(agentID)
	credHash := security.CredentialHash(credential)

	// Store enrolled agent.
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

	// Read CA cert for agent trust store.
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
