package main

import (
	"bufio"
	"crypto/rand"
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
)

// registrationTimeout is how long the server waits for the agent's
// initial registration message after the WebSocket handshake.
const registrationTimeout = 30 * time.Second

// Agent represents a connected remote agent.
type Agent struct {
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
	conn          net.Conn
	mu            sync.Mutex
}

// Server manages agents and viewers.
type Server struct {
	agents  map[string]*Agent
	viewers map[string]net.Conn
	mu      sync.RWMutex
	webDir  string
}

// NewServer creates a new Server instance.
func NewServer(webDir string) *Server {
	return &Server{
		agents:  make(map[string]*Agent),
		viewers: make(map[string]net.Conn),
		webDir:  webDir,
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

// generateID returns a random hex string.
func generateID() string {
	b := make([]byte, 8)
	rand.Read(b) //nolint:errcheck
	return fmt.Sprintf("%x", b)
}

// handleAgent manages the lifecycle of an agent connection.
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

	displayCount := reg.DisplayCount
	if displayCount < 1 {
		displayCount = 1
	}

	agent := &Agent{
		ID:            generateID(),
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
		conn:          conn,
	}

	s.mu.Lock()
	s.agents[agent.ID] = agent
	s.mu.Unlock()

	log.Printf("Agent registered: %s (%s) - %s/%s", agent.Name, agent.ID, agent.OS, agent.Arch)

	respPayload, _ := json.Marshal(map[string]string{"id": agent.ID})
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
		case protocol.OpText:
			var m protocol.Message
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}

			switch m.Type {
			case "screen", "display_switched":
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
func (s *Server) handleViewer(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Access-Control-Allow-Origin", "*")

	s.mu.RLock()
	agents := make([]Agent, 0, len(s.agents))
	for _, a := range s.agents {
		agents = append(agents, Agent{
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
		})
	}
	s.mu.RUnlock()

	json.NewEncoder(w).Encode(agents) //nolint:errcheck
}
