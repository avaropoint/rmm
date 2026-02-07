package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/avaropoint/rmm/internal/protocol"
	"github.com/avaropoint/rmm/internal/security"
)

// handleViewer manages the lifecycle of a viewer connection.
// Requires valid API key via "token" query parameter.
func (s *Server) handleViewer(w http.ResponseWriter, r *http.Request) {
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

	s.viewerInputLoop(agent, reader)
}

// viewerInputLoop reads viewer input and forwards it to the target agent.
func (s *Server) viewerInputLoop(agent *LiveAgent, reader *bufio.Reader) {
	for {
		opcode, data, err := protocol.ReadFrame(reader)
		if err != nil || opcode == protocol.OpClose {
			break
		}

		if opcode != protocol.OpText {
			continue
		}

		var m protocol.Message
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}

		if m.Type == "input" || m.Type == "switch_display" {
			agent.mu.Lock()
			_ = protocol.WriteServerFrame(agent.conn, protocol.OpText, data)
			agent.mu.Unlock()
		}
	}
}
