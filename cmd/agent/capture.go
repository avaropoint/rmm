package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/avaropoint/rmm/internal/protocol"
)

const (
	// captureInterval controls the target frame rate for screen capture.
	captureInterval = 100 * time.Millisecond // ~10 FPS

	// jpegQuality sets the JPEG compression level for screen captures.
	jpegQuality = 70

	// testPatternWidth and testPatternHeight define the fallback test image size.
	testPatternWidth  = 800
	testPatternHeight = 600
)

// Cached display count (computed once on first call).
var (
	cachedDisplayCount int
	displayCountOnce   sync.Once
)

// startCapture begins the screen-capture loop in a background goroutine.
func (a *Agent) startCapture() {
	a.captureMu.Lock()
	if a.capturing {
		a.captureMu.Unlock()
		return
	}
	a.capturing = true
	a.stopCapture = make(chan struct{})
	a.captureMu.Unlock()

	log.Println("Starting screen capture")

	go func() {
		ticker := time.NewTicker(captureInterval)
		defer ticker.Stop()

		for {
			select {
			case <-a.stopCapture:
				return
			case <-ticker.C:
				data, err := captureScreen(a.currentDisplay)
				if err != nil {
					continue
				}

				screenData, _ := json.Marshal(map[string]interface{}{
					"data": base64.StdEncoding.EncodeToString(data),
				})

				a.sendMessage(protocol.Message{
					Type:    "screen",
					Payload: screenData,
				})
			}
		}
	}()
}

// stopCaptureLoop signals the capture goroutine to stop.
func (a *Agent) stopCaptureLoop() {
	a.captureMu.Lock()
	defer a.captureMu.Unlock()

	if a.capturing && a.stopCapture != nil {
		close(a.stopCapture)
		a.capturing = false
		log.Println("Stopped screen capture")
	}
}

// handleSwitchDisplay processes a display-switch request from the viewer.
func (a *Agent) handleSwitchDisplay(payload json.RawMessage) {
	var req struct {
		Display int `json:"display"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		log.Printf("Failed to parse switch_display payload: %v", err)
		return
	}

	displayCount := getDisplayCount()
	if req.Display < 1 || req.Display > displayCount {
		log.Printf("Invalid display number %d (have %d displays)", req.Display, displayCount)
		return
	}

	a.captureMu.Lock()
	a.currentDisplay = req.Display
	a.captureMu.Unlock()

	log.Printf("Switched to display %d", req.Display)

	respData, _ := json.Marshal(map[string]interface{}{
		"display":       req.Display,
		"display_count": displayCount,
	})
	a.sendMessage(protocol.Message{
		Type:    "display_switched",
		Payload: respData,
	})
}

// captureScreen dispatches to the platform-specific capture implementation.
func captureScreen(display int) ([]byte, error) {
	switch runtime.GOOS {
	case "darwin":
		return captureScreenMacOS(display)
	case "linux":
		return captureScreenLinux()
	case "windows":
		return captureScreenWindows()
	default:
		return generateTestPattern()
	}
}

// getDisplayCount returns the number of connected displays.
// The result is cached because shelling out to system_profiler is expensive.
func getDisplayCount() int {
	displayCountOnce.Do(func() {
		if runtime.GOOS != "darwin" {
			cachedDisplayCount = 1
			return
		}
		cmd := exec.Command("system_profiler", "SPDisplaysDataType")
		output, err := cmd.Output()
		if err != nil {
			cachedDisplayCount = 1
			return
		}
		count := 0
		for _, line := range bytes.Split(output, []byte("\n")) {
			if bytes.Contains(line, []byte("Resolution:")) {
				count++
			}
		}
		if count == 0 {
			count = 1
		}
		cachedDisplayCount = count
	})
	return cachedDisplayCount
}

func captureScreenMacOS(display int) ([]byte, error) {
	tmpFile := fmt.Sprintf("/tmp/screen_%d.jpg", time.Now().UnixNano())
	defer os.Remove(tmpFile)

	displayArg := fmt.Sprintf("%d", display)
	cmd := exec.Command("screencapture", "-x", "-t", "jpg", "-C", "-D", displayArg, tmpFile)
	if err := cmd.Run(); err != nil {
		return generateTestPattern()
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return generateTestPattern()
	}
	return data, nil
}

func captureScreenLinux() ([]byte, error) {
	tmpFile := fmt.Sprintf("/tmp/screen_%d.jpg", time.Now().UnixNano())
	defer os.Remove(tmpFile)

	cmd := exec.Command("gnome-screenshot", "-f", tmpFile)
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("scrot", "-o", tmpFile)
		if err := cmd.Run(); err != nil {
			cmd = exec.Command("import", "-window", "root", tmpFile)
			if err := cmd.Run(); err != nil {
				return generateTestPattern()
			}
		}
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return generateTestPattern()
	}
	return data, nil
}

func captureScreenWindows() ([]byte, error) {
	tmpFile := fmt.Sprintf("%s\\screen_%d.jpg", os.TempDir(), time.Now().UnixNano())
	defer os.Remove(tmpFile)

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$screen = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
$bitmap = New-Object System.Drawing.Bitmap($screen.Width, $screen.Height)
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$graphics.CopyFromScreen($screen.Location, [System.Drawing.Point]::Empty, $screen.Size)
$bitmap.Save('%s', [System.Drawing.Imaging.ImageFormat]::Jpeg)
$graphics.Dispose()
$bitmap.Dispose()
`, tmpFile)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	if err := cmd.Run(); err != nil {
		return generateTestPattern()
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return generateTestPattern()
	}
	return data, nil
}

// generateTestPattern creates a simple test image when capture fails.
// Uses direct pixel buffer writes (4x faster than img.Set per-pixel).
func generateTestPattern() ([]byte, error) {
	const width, height = testPatternWidth, testPatternHeight
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	pix := img.Pix
	stride := img.Stride

	// Gradient background
	for y := 0; y < height; y++ {
		g := uint8(50 + (y * 100 / height))
		off := y * stride
		for x := 0; x < width; x++ {
			i := off + x*4
			pix[i+0] = uint8(50 + (x * 100 / width)) // R
			pix[i+1] = g                             // G
			pix[i+2] = 100                           // B
			pix[i+3] = 255                           // A
		}
	}

	// Grid lines
	for x := 0; x < width; x += 50 {
		for y := 0; y < height; y++ {
			i := y*stride + x*4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = 255, 255, 255, 100
		}
	}
	for y := 0; y < height; y += 50 {
		off := y * stride
		for x := 0; x < width; x++ {
			i := off + x*4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = 255, 255, 255, 100
		}
	}

	// Moving dot (progress indicator)
	t := time.Now().Second()
	cx := (t * width) / 60
	for dy := -5; dy <= 5; dy++ {
		for dx := -5; dx <= 5; dx++ {
			if dx*dx+dy*dy <= 25 {
				px, py := cx+dx, height/2+dy
				if px >= 0 && px < width && py >= 0 && py < height {
					i := py*stride + px*4
					pix[i], pix[i+1], pix[i+2], pix[i+3] = 255, 100, 100, 255
				}
			}
		}
	}

	var buf bytes.Buffer
	buf.Grow(width * height / 4) // Pre-size for ≈JPEG output
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
