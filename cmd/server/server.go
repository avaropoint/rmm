// Package main implements the management server that brokers connections
// between remote agents and browser-based viewers.
//
// Files in this package:
//   - server.go       — Server struct, LiveAgent, constants
//   - main.go         — Entry point, flag parsing, TLS mode selection
//   - websocket.go    — RFC 6455 WebSocket upgrade
//   - handler_agent.go  — Agent connection lifecycle
//   - handler_viewer.go — Viewer connection lifecycle
//   - handler_api.go    — REST API (agents, enrollment, auth)
package main

import (
	"net"
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

// newLiveAgent creates a LiveAgent from an enrollment record and registration data.
func newLiveAgent(enrolled *store.AgentRecord, reg *protocol.Registration, remoteAddr string, displayCount int, conn net.Conn) *LiveAgent {
	return &LiveAgent{
		ID:            enrolled.ID,
		Name:          reg.Name,
		Hostname:      reg.Hostname,
		OS:            reg.OS,
		OSVersion:     reg.OSVersion,
		Arch:          reg.Arch,
		IP:            remoteAddr,
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
}
