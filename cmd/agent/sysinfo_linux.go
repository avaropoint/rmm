//go:build linux

package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/avaropoint/rmm/internal/protocol"
)

// collectPlatformInfo gathers Linux-specific system details.
func collectPlatformInfo(info *SystemInfo) {
	info.OSVersion = linuxOSVersion()
	info.MemoryTotal, info.MemoryFree = linuxMemory()
	info.DiskTotal, info.DiskFree = diskUsage("/")
	info.Displays = linuxDisplays()
	info.UptimeSeconds = linuxUptime()
}

// linuxOSVersion reads /etc/os-release for a friendly name.
func linuxOSVersion() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "Linux"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			val := strings.TrimPrefix(line, "PRETTY_NAME=")
			return strings.Trim(val, "\"")
		}
	}
	return "Linux"
}

// linuxMemory reads /proc/meminfo for total and available memory.
func linuxMemory() (total, free uint64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseUint(fields[1], 10, 64)
		val *= 1024 // /proc/meminfo reports in kB

		switch fields[0] {
		case "MemTotal:":
			total = val
		case "MemAvailable:":
			free = val
		}
	}
	return total, free
}

// linuxDisplays queries xrandr for connected display resolutions.
func linuxDisplays() []protocol.DisplayInfo {
	out, err := exec.Command("xrandr", "--query").Output()
	if err != nil {
		return nil
	}

	var displays []protocol.DisplayInfo
	idx := 1
	for _, line := range strings.Split(string(out), "\n") {
		// Lines like: "DP-1 connected primary 2560x1440+0+0 ..."
		if !strings.Contains(line, " connected") {
			continue
		}
		fields := strings.Fields(line)
		for _, f := range fields {
			if w, h, ok := parseXrandrRes(f); ok {
				displays = append(displays, protocol.DisplayInfo{Index: idx, Width: w, Height: h})
				idx++
				break
			}
		}
	}
	return displays
}

// parseXrandrRes parses a "WxH+X+Y" token from xrandr output.
func parseXrandrRes(s string) (int, int, bool) {
	// Must contain "x" and "+" to be a geometry token
	xIdx := strings.Index(s, "x")
	pIdx := strings.Index(s, "+")
	if xIdx < 1 || pIdx < 1 || pIdx <= xIdx {
		return 0, 0, false
	}
	w, errW := strconv.Atoi(s[:xIdx])
	h, errH := strconv.Atoi(s[xIdx+1 : pIdx])
	if errW != nil || errH != nil {
		return 0, 0, false
	}
	return w, h, true
}

// linuxUptime reads /proc/uptime.
func linuxUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0
	}
	sec, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(sec)
}
