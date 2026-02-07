// Command server runs the management server that brokers connections
// between remote agents and browser-based viewers.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/avaropoint/rmm/internal/security"
	"github.com/avaropoint/rmm/internal/store"
	"github.com/avaropoint/rmm/internal/version"
)

func main() {
	addr := flag.String("addr", ":8443", "Server listen address")
	webDir := flag.String("web", "", "Web assets directory path")
	dataDir := flag.String("data", "data", "Data directory for database and certs")
	insecure := flag.Bool("insecure", false, "Run without TLS (development only)")
	flag.Parse()

	log.Printf("Server v%s (built %s)", version.Version, version.BuildTime)

	// Ensure data directory exists.
	if err := os.MkdirAll(*dataDir, 0700); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Initialise platform identity.
	platform, err := security.LoadOrCreatePlatform(*dataDir)
	if err != nil {
		log.Fatalf("Platform key: %v", err)
	}
	log.Printf("Platform fingerprint: %s", platform.Fingerprint())

	// Initialise TLS.
	var tlsCfg *tls.Config
	var tlsPaths *security.TLSConfig
	if !*insecure {
		tlsCfg, tlsPaths, err = security.LoadOrGenerateTLS(*dataDir)
		if err != nil {
			log.Fatalf("TLS: %v", err)
		}
		log.Printf("TLS certificates ready (%s)", tlsPaths.CertPath)
	}

	// Open database.
	dbPath := filepath.Join(*dataDir, "platform.db")
	db, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		log.Fatalf("Database: %v", err)
	}
	defer db.Close() //nolint:errcheck

	// Ensure at least one API key exists (first-run setup).
	ensureAdminKey(db)

	// Resolve web directory.
	if *webDir == "" {
		*webDir = findWebDir()
	}
	if *webDir == "" {
		log.Fatal("Web directory not found. Use -web flag to specify the path.")
	}
	absWebDir, _ := filepath.Abs(*webDir)
	log.Printf("Web directory: %s", absWebDir)

	srv := NewServer(absWebDir, db, platform, tlsPaths)

	auth := security.NewAuthMiddleware(db)

	// Public endpoints (no auth required).
	http.HandleFunc("/api/enroll", srv.handleEnroll)
	http.HandleFunc("/ws/agent", srv.handleAgent)
	http.HandleFunc("/api/auth/verify", srv.handleAuthVerify)

	// Authenticated endpoints.
	http.HandleFunc("/api/agents", auth.Wrap(srv.handleListAgents))
	http.HandleFunc("/api/enrollment", auth.Wrap(srv.handleEnrollmentTokens))
	http.HandleFunc("/ws/viewer", srv.handleViewer)

	// Static files.
	http.Handle("/", http.FileServer(http.Dir(absWebDir)))

	if *insecure {
		log.Printf("WARNING: Running without TLS (development mode)")
		log.Printf("Dashboard: http://localhost%s", *addr)
		log.Fatal(http.ListenAndServe(*addr, nil))
	} else {
		log.Printf("Dashboard: https://localhost%s", *addr)
		server := &http.Server{
			Addr:      *addr,
			TLSConfig: tlsCfg,
		}
		log.Fatal(server.ListenAndServeTLS("", ""))
	}
}

// ensureAdminKey creates the initial admin API key if none exist.
func ensureAdminKey(db store.Store) {
	keys, err := db.ListAPIKeys(context.TODO())
	if err != nil {
		log.Fatalf("Check API keys: %v", err)
	}
	if len(keys) > 0 {
		return
	}

	apiKey, rawKey, err := security.GenerateAPIKey("admin")
	if err != nil {
		log.Fatalf("Generate admin key: %v", err)
	}
	if err := db.CreateAPIKey(context.TODO(), apiKey); err != nil {
		log.Fatalf("Store admin key: %v", err)
	}

	log.Println("==========================================================")
	log.Println("  INITIAL ADMIN API KEY (save this — shown only once):")
	log.Printf("  %s", rawKey)
	log.Println("==========================================================")
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
