package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"webterminal/handlers"
	"webterminal/middleware"
)

//go:embed static
var staticFiles embed.FS

const shutdownTimeout = 30 * time.Second

func main() {
	path := configPath()

	cfg := defaultConfig()
	loadConfig(path, cfg)

	var showHelp, genConfig, manageService bool
	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flags.Usage = func() { writeUsage(flags.Output(), path) }
	flags.BoolVar(&showHelp, "help", false, "show this help and exit")
	flags.BoolVar(&showHelp, "h", false, "show this help and exit")
	flags.BoolVar(&genConfig, "genconfig", false, "write a default config next to the binary and exit")
	flags.BoolVar(&manageService, "service", false, "create the systemd unit, or remove it if it exists")
	bindFlags(flags, cfg)
	flags.Parse(os.Args[1:])

	if showHelp {
		writeUsage(os.Stdout, path)
		return
	}

	if genConfig && manageService {
		log.Fatalf("--genconfig and --service cannot be combined")
	}

	if genConfig {
		if err := writeConfig(path); err != nil {
			log.Fatalf("Cannot generate config: %v", err)
		}
		log.Printf("Wrote %s", path)
		return
	}

	if manageService {
		if err := toggleService(); err != nil {
			log.Fatalf("Cannot manage %s: %v", serviceName, err)
		}
		return
	}

	serve(cfg)
}

func serve(cfg *Config) {
	assets, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("Cannot open embedded assets: %v", err)
	}
	page, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		log.Fatalf("Cannot read embedded index page: %v", err)
	}

	bruteForce := middleware.NewBruteForce(int(cfg.MaxAttempts), time.Duration(cfg.BanDuration))

	auth := handlers.NewAuthHandler(bruteForce, handlers.Credentials{
		Login:       string(cfg.Login),
		Password:    string(cfg.Password),
		Fail2banLog: string(cfg.Fail2banLog),
		SessionTTL:  time.Duration(cfg.SessionTTL),
	})
	terminal := handlers.NewTerminalHandler(string(cfg.TerminalDir))
	files := handlers.NewFileHandler("/")
	metrics := handlers.NewMetricsHandler()

	mux := http.NewServeMux()

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(assets))))

	mux.HandleFunc("/api/login", auth.Login)
	mux.HandleFunc("/api/logout", auth.Logout)
	mux.HandleFunc("/api/terminal", auth.Require(terminal.HandleWebSocket))
	mux.HandleFunc("/api/metrics", auth.Require(metrics.GetMetrics))
	mux.HandleFunc("/api/files", auth.Require(files.ListFiles))
	mux.HandleFunc("/api/files/upload", auth.Require(files.UploadFile))
	mux.HandleFunc("/api/files/download", auth.Require(files.DownloadFile))
	mux.HandleFunc("/api/files/download-folder", auth.Require(files.DownloadFolder))
	mux.HandleFunc("/api/files/mkdir", auth.Require(files.CreateFolder))
	mux.HandleFunc("/api/files/delete", auth.Require(files.DeleteItem))

	mux.HandleFunc("/", handlers.IndexHandler(page))

	server := &http.Server{
		Addr:              string(cfg.ListenAddr),
		Handler:           mux,
		ReadHeaderTimeout: time.Duration(cfg.ReadHeaderTimeout),
		IdleTimeout:       time.Duration(cfg.IdleTimeout),
	}

	go func() {
		log.Printf("Starting web terminal on %s", cfg.ListenAddr)
		warnDefaultPassword(cfg)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func warnDefaultPassword(cfg *Config) {
	if cfg.Password != defaultConfig().Password || isLoopbackAddr(string(cfg.ListenAddr)) {
		return
	}
	log.Printf("WARNING: the default password is in use and %s is reachable from the network. "+
		"Set a password in %s or pass --password.", cfg.ListenAddr, configName)
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
