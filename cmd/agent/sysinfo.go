package main

import (
	"encoding/json"
	"net"
	"os"
	"os/user"
	"runtime"
	"time"

	"github.com/avaropoint/rmm/internal/protocol"
	"github.com/avaropoint/rmm/internal/version"
)

// SystemInfo holds everything we can discover about the host device.
// The struct is serialised directly to JSON for the registration payload
// and can be refreshed on demand without changing the message format.
type SystemInfo struct {
	Name          string                 `json:"name"`
	Hostname      string                 `json:"hostname"`
	OS            string                 `json:"os"`
	OSVersion     string                 `json:"os_version"`
	Arch          string                 `json:"arch"`
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
}

// CollectSystemInfo gathers device information using stdlib and
// platform-native commands. No third-party tools are required.
func CollectSystemInfo(name string) SystemInfo {
	hostname := getHostname()
	if name == "" {
		name = hostname
	}

	info := SystemInfo{
		Name:         name,
		Hostname:     hostname,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		CPUCount:     runtime.NumCPU(),
		AgentVersion: version.Version,
	}

	// Platform-specific collection
	collectPlatformInfo(&info)

	// Cross-platform: local IPs (pure stdlib)
	info.LocalIPs = collectLocalIPs()

	// Cross-platform: current user
	if u, err := user.Current(); err == nil {
		info.Username = u.Username
	}

	// Ensure display count matches the displays slice
	if len(info.Displays) > 0 {
		info.DisplayCount = len(info.Displays)
	}
	if info.DisplayCount < 1 {
		info.DisplayCount = 1
	}

	return info
}

// PrimaryDisplay returns the first display, or a sensible default.
func (s *SystemInfo) PrimaryDisplay() protocol.DisplayInfo {
	if len(s.Displays) > 0 {
		return s.Displays[0]
	}
	return protocol.DisplayInfo{Index: 1, Width: 0, Height: 0}
}

// ToJSON serialises the system info for use as a message payload.
func (s *SystemInfo) ToJSON() json.RawMessage {
	data, _ := json.Marshal(s)
	return data
}

// collectLocalIPs returns all non-loopback unicast IPv4/IPv6 addresses.
func collectLocalIPs() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := extractIP(addr)
			if ip != "" {
				ips = append(ips, ip)
			}
		}
	}
	return ips
}

// extractIP returns the string form of a non-loopback, non-link-local address.
func extractIP(addr net.Addr) string {
	switch v := addr.(type) {
	case *net.IPNet:
		if v.IP.IsLoopback() || v.IP.IsLinkLocalUnicast() {
			return ""
		}
		return v.IP.String()
	case *net.IPAddr:
		if v.IP.IsLoopback() || v.IP.IsLinkLocalUnicast() {
			return ""
		}
		return v.IP.String()
	}
	return ""
}

// getHostname returns the system hostname or "unknown".
func getHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

// bootTimeToUptime converts a boot Unix timestamp (seconds) to uptime.
func bootTimeToUptime(bootEpoch int64) int64 {
	return time.Now().Unix() - bootEpoch
}
