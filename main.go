package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"github.com/lab1702/netrek-web/server"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

func main() {
	port := flag.String("port", "8080", "Server port")
	flag.Parse()

	log.Printf("Starting Netrek Web Server on port %s", *port)

	// Create game server
	gameServer := server.NewServer()
	go gameServer.Run()

	// Serve static files from the static subdirectory
	fsys, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/", http.FileServer(http.FS(fsys)))

	// WebSocket endpoint
	http.HandleFunc("/ws", gameServer.HandleWebSocket)

	// Team stats endpoint
	http.HandleFunc("/api/teams", gameServer.HandleTeamStats)

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Shutdown API endpoint
	shutdownChan := make(chan struct{})
	http.HandleFunc("/api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		log.Println("Shutdown requested via API")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Server shutting down"))
		go func() {
			time.Sleep(100 * time.Millisecond) // Small delay to allow response to be sent
			close(shutdownChan)
		}()
	})

	// Start HTTP server
	srv := &http.Server{
		Addr:         ":" + *port,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Server running at http://localhost:%s", *port)

	// Start server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal (either from API or OS signal)
	select {
	case <-shutdownChan:
		log.Println("Shutting down server (API request)...")
	case sig := <-sigChan:
		log.Printf("Shutting down server (signal: %v)...", sig)
	}

	// Create a context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown the HTTP server gracefully
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
	os.Exit(0)
}
