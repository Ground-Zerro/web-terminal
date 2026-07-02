package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"webterminal/handlers"
	"webterminal/middleware"
)

func main() {
	// Load configuration
	cfg := loadConfig()

	// Create middleware
	bruteForce := middleware.NewBruteForce(cfg.MaxAttempts, cfg.BanDuration)

	// Create handlers
	authHandler := handlers.NewAuthHandler(bruteForce, cfg.Login, cfg.Password, cfg.Fail2banLog)
	terminalHandler := handlers.NewTerminalHandler(cfg.TerminalDir)
	fileHandler := handlers.NewFileHandler()
	metricsHandler := handlers.NewMetricsHandler()

	// Setup routes
	mux := http.NewServeMux()

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// API endpoints
	mux.HandleFunc("/api/login", authHandler.Login)
	mux.HandleFunc("/api/logout", authHandler.Logout)
	mux.HandleFunc("/api/terminal", terminalHandler.HandleWebSocket)
	mux.HandleFunc("/api/files", fileHandler.ListFiles)
	mux.HandleFunc("/api/files/upload", fileHandler.UploadFile)
	mux.HandleFunc("/api/files/download", fileHandler.DownloadFile)
	mux.HandleFunc("/api/files/mkdir", fileHandler.CreateFolder)
	mux.HandleFunc("/api/files/delete", fileHandler.DeleteItem)
	mux.HandleFunc("/api/files/download-folder", fileHandler.DownloadFolder)
	mux.HandleFunc("/api/metrics", metricsHandler.GetMetrics)

	// Main page
	mux.HandleFunc("/", handlers.IndexHandler)

	// Create server
	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// Start server
	go func() {
		log.Printf("Starting web terminal on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func init() {
	// Set working directory (will be overridden by config if present)
	if err := os.Chdir("/root/terminal"); err != nil {
		log.Printf("Warning: Could not change to /root/terminal: %v", err)
	}
}
