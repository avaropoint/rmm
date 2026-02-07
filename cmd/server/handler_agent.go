package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/avaropoint/rmm/internal/protocol"
	"github.com/avaropoint/rmm/internal/security"
)

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

	agent := newLiveAgent(enrolled, &reg, r.RemoteAddr, displayCount, conn)

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

	s.agentMessageLoop(agent, reader, conn)
}

// agentMessageLoop reads and dispatches messages from an agent connection.
func (s *Server) agentMessageLoop(agent *LiveAgent, reader *bufio.Reader, conn net.Conn) {
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
			s.mu.RLock()
			if vc, ok := s.viewers[agent.ID]; ok {
				_ = protocol.WriteServerFrame(vc, protocol.OpBinary, data)
			}
			s.mu.RUnlock()
		case protocol.OpText:
			s.handleAgentTextMessage(agent, data)
		}
	}
}

// handleAgentTextMessage processes a text message from an agent.
func (s *Server) handleAgentTextMessage(agent *LiveAgent, data []byte) {
	var m protocol.Message
	if err := json.Unmarshal(data, &m); err != nil {
		return
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
