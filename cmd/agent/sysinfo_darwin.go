//go:build darwin

package main

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"

	"github.com/avaropoint/rmm/internal/protocol"
)

// collectPlatformInfo gathers macOS-specific system details.
func collectPlatformInfo(info *SystemInfo) {
	info.OSVersion = macOSVersion()
	info.MemoryTotal, info.MemoryFree = macOSMemory()
	info.DiskTotal, info.DiskFree = diskUsage("/")
	info.Displays = macOSDisplays()
	info.UptimeSeconds = macOSUptime()
}

// macOSVersion returns a string like "macOS 15.2".
func macOSVersion() string {
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return "macOS"
	}
	return "macOS " + strings.TrimSpace(string(out))
}

// macOSMemory reads total and free (available) memory.
func macOSMemory() (total, free uint64) {
	total = sysctlUint64("hw.memsize")

	// vm_stat gives pages; page size from hw.pagesize
	pageSize := sysctlUint64("hw.pagesize")
	if pageSize == 0 {
		pageSize = 16384 // Apple Silicon default page size
	}

	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		return total, 0
	}

	var freePages, inactivePages uint64
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Pages free:") {
			freePages = parseVMStatValue(line)
		} else if strings.HasPrefix(line, "Pages inactive:") {
			inactivePages = parseVMStatValue(line)
		}
	}
	free = (freePages + inactivePages) * pageSize
	return total, free
}

func parseVMStatValue(line string) uint64 {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return 0
	}
	s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[1]), "."))
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

// macOSDisplays reads per-display resolution from system_profiler.
func macOSDisplays() []protocol.DisplayInfo {
	out, err := exec.Command("system_profiler", "SPDisplaysDataType").Output()
	if err != nil {
		return nil
	}

	var displays []protocol.DisplayInfo
	idx := 1
	for _, line := range bytes.Split(out, []byte("\n")) {
		l := strings.TrimSpace(string(line))
		// Lines like: "Resolution: 2560 x 1440 (QHD/WQHD)"
		// or:         "Resolution: 3456 x 2234 Retina"
		if strings.HasPrefix(l, "Resolution:") {
			w, h := parseResolution(l)
			if w > 0 && h > 0 {
				displays = append(displays, protocol.DisplayInfo{
					Index:  idx,
					Width:  w,
					Height: h,
				})
				idx++
			}
		}
	}
	return displays
}

// parseResolution extracts width and height from a system_profiler line.
func parseResolution(line string) (int, int) {
	// Strip prefix
	line = strings.TrimPrefix(line, "Resolution:")
	line = strings.TrimSpace(line)

	// Split on " x "
	parts := strings.SplitN(line, " x ", 2)
	if len(parts) != 2 {
		return 0, 0
	}

	w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))

	// Height is the first numeric token in the second part
	hStr := strings.Fields(parts[1])
	if len(hStr) == 0 {
		return 0, 0
	}
	h, errH := strconv.Atoi(hStr[0])

	if errW != nil || errH != nil {
		return 0, 0
	}
	return w, h
}

// macOSUptime reads uptime via sysctl kern.boottime.
func macOSUptime() int64 {
	out, err := exec.Command("sysctl", "-n", "kern.boottime").Output()
	if err != nil {
		return 0
	}
	// Output: "{ sec = 1707100000, usec = 0 } Thu Feb ..."
	s := string(out)
	const prefix = "sec = "
	idx := strings.Index(s, prefix)
	if idx < 0 {
		return 0
	}
	s = s[idx+len(prefix):]
	comma := strings.Index(s, ",")
	if comma < 0 {
		return 0
	}
	bootSec, err := strconv.ParseInt(s[:comma], 10, 64)
	if err != nil {
		return 0
	}
	return bootTimeToUptime(bootSec)
}

// sysctlUint64 reads a numeric sysctl value using the sysctl CLI.
// We avoid syscall.Sysctl because it strips trailing null bytes from
// binary values, corrupting numbers like hw.memsize.
func sysctlUint64(name string) uint64 {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	return v
}
