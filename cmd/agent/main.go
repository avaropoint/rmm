// Command agent runs the remote desktop agent that connects to the server,
// captures the screen, and forwards input events.
package main

import (
	"flag"
	"log"
	"runtime"
	"time"

	"github.com/avaropoint/rmm/internal/version"
)

// reconnectDelay is the pause between connection attempts.
const reconnectDelay = 5 * time.Second

func main() {
	serverURL := flag.String("server", "ws://localhost:8080", "Server WebSocket URL")
	name := flag.String("name", "", "Agent name (defaults to hostname)")
	flag.Parse()

	log.Printf("Agent v%s (built %s)", version.Version, version.BuildTime)
	log.Printf("OS: %s, Arch: %s", runtime.GOOS, runtime.GOARCH)
	log.Printf("Server: %s", *serverURL)

	agent := &Agent{
		serverURL: *serverURL,
		name:      *name,
	}

	for {
		if err := agent.run(); err != nil {
			log.Printf("Connection error: %v", err)
		}
		log.Printf("Reconnecting in %s...", reconnectDelay)
		time.Sleep(reconnectDelay)
	}
}
