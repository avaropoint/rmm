// Package protocol defines the shared message types and constants
// used for communication between the server and agents.
package protocol

import "encoding/json"

// WebSocket opcodes per RFC 6455.
const (
	OpContinue = 0
	OpText     = 1
	OpBinary   = 2
	OpClose    = 8
	OpPing     = 9
	OpPong     = 10
)

// Message is the envelope for all WebSocket messages exchanged
// between agents, the server, and viewers.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// DisplayInfo describes a single connected display.
// Used by both the agent (collection) and the server (API response).
type DisplayInfo struct {
	Index  int `json:"index"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Registration is the wire format sent by the agent during registration.
// Shared between agent (serialisation) and server (deserialisation) to
// keep the two sides in sync.
type Registration struct {
	Name          string        `json:"name"`
	Hostname      string        `json:"hostname"`
	OS            string        `json:"os"`
	OSVersion     string        `json:"os_version"`
	Arch          string        `json:"arch"`
	CPUCount      int           `json:"cpu_count"`
	MemoryTotal   uint64        `json:"memory_total"`
	MemoryFree    uint64        `json:"memory_free"`
	DiskTotal     uint64        `json:"disk_total"`
	DiskFree      uint64        `json:"disk_free"`
	Displays      []DisplayInfo `json:"displays"`
	DisplayCount  int           `json:"display_count"`
	LocalIPs      []string      `json:"local_ips"`
	Username      string        `json:"username"`
	UptimeSeconds int64         `json:"uptime_seconds"`
	AgentVersion  string        `json:"agent_version"`
}
