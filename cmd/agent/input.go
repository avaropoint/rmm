package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"runtime"
)

// handleInput parses an input message and dispatches to the
// appropriate mouse or keyboard handler.
func (a *Agent) handleInput(payload json.RawMessage) {
	var input struct {
		Kind   string `json:"kind"`
		Action string `json:"action"`
		X      int    `json:"x"`
		Y      int    `json:"y"`
		Button int    `json:"button"`
		Key    string `json:"key"`
		Code   int    `json:"code"`
	}
	if err := json.Unmarshal(payload, &input); err != nil {
		return
	}

	switch input.Kind {
	case "mouse":
		injectMouse(input.Action, input.X, input.Y, input.Button)
	case "key":
		injectKey(input.Action, input.Key, input.Code)
	}
}

// injectMouse dispatches mouse events to the platform handler.
func injectMouse(action string, x, y, button int) {
	switch runtime.GOOS {
	case "darwin":
		injectMouseDarwin(action, x, y, button)
	case "linux":
		injectMouseLinux(action, x, y, button)
	case "windows":
		injectMouseWindows(action, x, y, button)
	default:
		log.Printf("Mouse injection not supported on %s", runtime.GOOS)
	}
}

// injectKey dispatches keyboard events to the platform handler.
func injectKey(action, key string, code int) {
	if action != "down" {
		return // Only inject on keydown to avoid double-typing.
	}
	switch runtime.GOOS {
	case "darwin":
		injectKeyDarwin(key, code)
	case "linux":
		injectKeyLinux(key, code)
	case "windows":
		injectKeyWindows(key, code)
	default:
		log.Printf("Key injection not supported on %s", runtime.GOOS)
	}
}

// ---------------------------------------------------------------------------
// macOS input injection (requires cliclick: brew install cliclick)
// ---------------------------------------------------------------------------

var (
	cliclickChecked   bool
	cliclickAvailable bool
	retinaScale       = 2 // macOS Retina displays use 2x scaling
)

func injectMouseDarwin(action string, x, y, button int) {
	if !cliclickChecked {
		cliclickChecked = true
		if _, err := exec.LookPath("cliclick"); err == nil {
			cliclickAvailable = true
			log.Println("Mouse control: cliclick found")
		} else {
			log.Println("WARNING: cliclick not found. Install with: brew install cliclick")
			log.Println("Then grant Accessibility permissions in System Preferences")
		}
	}
	if !cliclickAvailable {
		return
	}

	scaledX := x / retinaScale
	scaledY := y / retinaScale

	var args []string
	switch action {
	case "move":
		args = []string{"m:" + fmt.Sprintf("%d,%d", scaledX, scaledY)}
	case "down":
		args = []string{"m:" + fmt.Sprintf("%d,%d", scaledX, scaledY)}
	case "up":
		if button == 2 {
			args = []string{"rc:" + fmt.Sprintf("%d,%d", scaledX, scaledY)}
		} else {
			args = []string{"c:" + fmt.Sprintf("%d,%d", scaledX, scaledY)}
		}
	}

	if len(args) > 0 {
		log.Printf("Running: cliclick %v (original: %d,%d)", args, x, y)
		cmd := exec.Command("cliclick", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("cliclick error: %v, output: %s", err, string(output))
		}
	}
}

func injectKeyDarwin(key string, _ int) {
	var script string

	switch key {
	case "Enter":
		script = `tell application "System Events" to key code 36`
	case "Tab":
		script = `tell application "System Events" to key code 48`
	case "Backspace":
		script = `tell application "System Events" to key code 51`
	case "Escape":
		script = `tell application "System Events" to key code 53`
	case "ArrowUp":
		script = `tell application "System Events" to key code 126`
	case "ArrowDown":
		script = `tell application "System Events" to key code 125`
	case "ArrowLeft":
		script = `tell application "System Events" to key code 123`
	case "ArrowRight":
		script = `tell application "System Events" to key code 124`
	case "Space", " ":
		script = `tell application "System Events" to key code 49`
	default:
		if len(key) == 1 {
			script = fmt.Sprintf(`tell application "System Events" to keystroke "%s"`, key)
		}
	}

	if script != "" {
		exec.Command("osascript", "-e", script).Run() //nolint:errcheck
	}
}

// ---------------------------------------------------------------------------
// Linux input injection (requires xdotool: apt install xdotool)
// ---------------------------------------------------------------------------

var (
	xdotoolChecked   bool
	xdotoolAvailable bool
)

func injectMouseLinux(action string, x, y, button int) {
	if !xdotoolChecked {
		xdotoolChecked = true
		if _, err := exec.LookPath("xdotool"); err == nil {
			xdotoolAvailable = true
			log.Println("Mouse control: xdotool found")
		} else {
			log.Println("WARNING: xdotool not found. Install with: sudo apt install xdotool")
		}
	}
	if !xdotoolAvailable {
		return
	}

	xs, ys := fmt.Sprintf("%d", x), fmt.Sprintf("%d", y)
	bs := fmt.Sprintf("%d", button+1)

	switch action {
	case "move":
		exec.Command("xdotool", "mousemove", xs, ys).Run() //nolint:errcheck
	case "down":
		exec.Command("xdotool", "mousemove", xs, ys).Run() //nolint:errcheck
		exec.Command("xdotool", "mousedown", bs).Run()     //nolint:errcheck
	case "up":
		exec.Command("xdotool", "mousemove", xs, ys).Run() //nolint:errcheck
		exec.Command("xdotool", "mouseup", bs).Run()       //nolint:errcheck
	}
}

func injectKeyLinux(key string, _ int) {
	if !xdotoolAvailable {
		return
	}
	xdoKey := key
	switch key {
	case "Enter":
		xdoKey = "Return"
	case "Backspace":
		xdoKey = "BackSpace"
	case "ArrowUp":
		xdoKey = "Up"
	case "ArrowDown":
		xdoKey = "Down"
	case "ArrowLeft":
		xdoKey = "Left"
	case "ArrowRight":
		xdoKey = "Right"
	case " ":
		xdoKey = "space"
	}
	exec.Command("xdotool", "key", xdoKey).Run() //nolint:errcheck
}

// ---------------------------------------------------------------------------
// Windows input injection (PowerShell)
// ---------------------------------------------------------------------------

// windowsMouseScript builds a PowerShell script that moves the cursor to (x, y)
// and optionally fires a mouse_event with the given flags (0 = move only).
func windowsMouseScript(x, y int, mouseEventFlag string) string {
	base := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point(%d, %d)
`, x, y)
	if mouseEventFlag == "" {
		return base
	}
	return base + fmt.Sprintf(`$signature = @"
[DllImport("user32.dll")]
public static extern void mouse_event(int dwFlags, int dx, int dy, int dwData, int dwExtraInfo);
"@
$mouse = Add-Type -MemberDefinition $signature -Name "MouseEvent" -Namespace "Win32" -PassThru
$mouse::mouse_event(%s, 0, 0, 0, 0)
`, mouseEventFlag)
}

func injectMouseWindows(action string, x, y, _ int) {
	var ps string
	switch action {
	case "move":
		ps = windowsMouseScript(x, y, "")
	case "down":
		ps = windowsMouseScript(x, y, "0x0002")
	case "up":
		ps = windowsMouseScript(x, y, "0x0004")
	}
	if ps != "" {
		exec.Command("powershell", "-Command", ps).Run() //nolint:errcheck
	}
}

func injectKeyWindows(key string, _ int) {
	sendKey := key
	switch key {
	case "Enter":
		sendKey = "{ENTER}"
	case "Tab":
		sendKey = "{TAB}"
	case "Backspace":
		sendKey = "{BACKSPACE}"
	case "Escape":
		sendKey = "{ESC}"
	case "ArrowUp":
		sendKey = "{UP}"
	case "ArrowDown":
		sendKey = "{DOWN}"
	case "ArrowLeft":
		sendKey = "{LEFT}"
	case "ArrowRight":
		sendKey = "{RIGHT}"
	case " ":
		sendKey = " "
	}

	ps := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.SendKeys]::SendWait("%s")
`, sendKey)
	exec.Command("powershell", "-Command", ps).Run() //nolint:errcheck
}
