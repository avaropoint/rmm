// Command server runs the web server that brokers connections
// between remote agents and browser-based viewers.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/avaropoint/rmm/internal/version"
)

func main() {
	addr := flag.String("addr", ":8080", "Server listen address")
	webDir := flag.String("web", "", "Web assets directory path")
	flag.Parse()

	log.Printf("Server v%s (built %s)", version.Version, version.BuildTime)

	if *webDir == "" {
		*webDir = findWebDir()
	}
	if *webDir == "" {
		log.Fatal("Web directory not found. Use -web flag to specify the path.")
	}

	absWebDir, _ := filepath.Abs(*webDir)
	log.Printf("Web directory: %s", absWebDir)

	srv := NewServer(absWebDir)

	http.HandleFunc("/api/agents", srv.handleListAgents)
	http.HandleFunc("/ws/agent", srv.handleAgent)
	http.HandleFunc("/ws/viewer", srv.handleViewer)
	http.Handle("/", http.FileServer(http.Dir(absWebDir)))

	log.Printf("Dashboard: http://localhost%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

// findWebDir searches common locations for the web assets directory.
func findWebDir() string {
	candidates := []string{
		"web",
		"../web",
		filepath.Join(os.Args[0], "..", "web"),
	}

	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), "web"),
			filepath.Join(filepath.Dir(exe), "..", "web"),
		)
	}

	for _, dir := range candidates {
		abs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(abs, "index.html")); err == nil {
			return abs
		}
	}

	return ""
}
