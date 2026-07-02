package main

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

// Config holds all configurable parameters
type Config struct {
	// Server
	ListenAddr string `json:"listen_addr"`

	// Auth
	Login    string `json:"login"`
	Password string `json:"password"`

	// Paths
	WorkDir      string `json:"work_dir"`
	TerminalDir  string `json:"terminal_dir"`
	Fail2banLog  string `json:"fail2ban_log"`

	// Brute force
	MaxAttempts int           `json:"max_attempts"`
	BanDuration time.Duration `json:"ban_duration"`

	// Timeouts
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
}

// defaultConfig returns the built-in defaults
func defaultConfig() *Config {
	return &Config{
		ListenAddr:   "127.0.0.1:8081",
		Login:        "admin",
		Password:     "root",
		WorkDir:      "/root/terminal",
		TerminalDir:  "/root",
		Fail2banLog:  "/var/log/webterminal-bruteforce.log",
		MaxAttempts:  6,
		BanDuration:  15 * time.Minute,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

// loadConfig tries to read config.json from the current working directory.
// Falls back to defaults for missing or invalid values.
func loadConfig() *Config {
	cfg := defaultConfig()

	configPath := "config.json"

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("No config.json found, using defaults")
		} else {
			log.Printf("Error reading config.json: %v, using defaults", err)
		}
		return cfg
	}

	// Parse JSON into a map to handle partial configs
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Printf("Invalid config.json: %v, using defaults", err)
		return cfg
	}

	log.Printf("Loaded config.json from %s", configPath)

	// Apply values only if present and valid
	if v, ok := raw["listen_addr"].(string); ok && v != "" {
		cfg.ListenAddr = v
	}
	if v, ok := raw["login"].(string); ok && v != "" {
		cfg.Login = v
	}
	if v, ok := raw["password"].(string); ok && v != "" {
		cfg.Password = v
	}
	if v, ok := raw["work_dir"].(string); ok && v != "" {
		cfg.WorkDir = v
	}
	if v, ok := raw["terminal_dir"].(string); ok && v != "" {
		cfg.TerminalDir = v
	}
	if v, ok := raw["fail2ban_log"].(string); ok && v != "" {
		cfg.Fail2banLog = v
	}
	if v, ok := raw["max_attempts"].(float64); ok && v > 0 {
		cfg.MaxAttempts = int(v)
	}
	if v, ok := raw["ban_duration"].(float64); ok && v > 0 {
		cfg.BanDuration = time.Duration(v) * time.Minute
	}
	if v, ok := raw["read_timeout"].(float64); ok && v > 0 {
		cfg.ReadTimeout = time.Duration(v) * time.Second
	}
	if v, ok := raw["write_timeout"].(float64); ok && v > 0 {
		cfg.WriteTimeout = time.Duration(v) * time.Second
	}
	if v, ok := raw["idle_timeout"].(float64); ok && v > 0 {
		cfg.IdleTimeout = time.Duration(v) * time.Second
	}

	return cfg
}
