//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/avaropoint/rmm/internal/protocol"
)

// collectPlatformInfo gathers Windows-specific system details.
func collectPlatformInfo(info *SystemInfo) {
	info.OSVersion = windowsOSVersion()
	info.MemoryTotal, info.MemoryFree = windowsMemory()
	info.DiskTotal, info.DiskFree = windowsDisk()
	info.Displays = windowsDisplays()
	info.UptimeSeconds = windowsUptime()
}

// windowsOSVersion reads the OS caption via PowerShell.
func windowsOSVersion() string {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		"(Get-CimInstance Win32_OperatingSystem).Caption").Output()
	if err != nil {
		return "Windows"
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "Windows"
	}
	return v
}

// windowsMemory reads total and free physical memory via WMI.
func windowsMemory() (total, free uint64) {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		"$os = Get-CimInstance Win32_OperatingSystem; "+
			"\"$($os.TotalVisibleMemorySize) $($os.FreePhysicalMemory)\"").Output()
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) >= 2 {
		t, _ := strconv.ParseUint(fields[0], 10, 64)
		f, _ := strconv.ParseUint(fields[1], 10, 64)
		total = t * 1024 // WMI reports in kB
		free = f * 1024
	}
	return total, free
}

// windowsDisk reads C: drive total and free space via WMI.
func windowsDisk() (total, free uint64) {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		"$d = Get-CimInstance Win32_LogicalDisk -Filter \"DeviceID='C:'\"; "+
			"\"$($d.Size) $($d.FreeSpace)\"").Output()
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) >= 2 {
		total, _ = strconv.ParseUint(fields[0], 10, 64)
		free, _ = strconv.ParseUint(fields[1], 10, 64)
	}
	return total, free
}

// windowsDisplays queries connected display resolutions via WMI.
func windowsDisplays() []protocol.DisplayInfo {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		"Get-CimInstance Win32_VideoController | ForEach-Object { "+
			"\"$($_.CurrentHorizontalResolution) $($_.CurrentVerticalResolution)\" }").Output()
	if err != nil {
		return nil
	}

	var displays []protocol.DisplayInfo
	idx := 1
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		w, errW := strconv.Atoi(fields[0])
		h, errH := strconv.Atoi(fields[1])
		if errW != nil || errH != nil || w == 0 || h == 0 {
			continue
		}
		displays = append(displays, protocol.DisplayInfo{Index: idx, Width: w, Height: h})
		idx++
	}
	return displays
}

// windowsUptime reads system uptime in seconds via WMI.
func windowsUptime() int64 {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		"([int](Get-CimInstance Win32_OperatingSystem).LastBootUpTime.Subtract("+
			"(Get-Date)).TotalSeconds) * -1").Output()
	if err != nil {
		return 0
	}
	sec, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return sec
}
