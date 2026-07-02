package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
	"webterminal/middleware"
)

// Session represents a user session
type Session struct {
	Token     string
	User      string
	CreatedAt time.Time
}

// AuthHandler handles authentication
type AuthHandler struct {
	bruteForce *middleware.BruteForce
	sessions   map[string]*Session
	mu         sync.RWMutex
	login      string
	password   string
	fail2banLog string
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(bruteForce *middleware.BruteForce, login, password, fail2banLog string) *AuthHandler {
	return &AuthHandler{
		bruteForce:  bruteForce,
		sessions:    make(map[string]*Session),
		login:       login,
		password:    password,
		fail2banLog: fail2banLog,
	}
}

// LoginRequest represents a login request
type LoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Login handles user login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get client IP
	ip := getClientIP(r)

	// Check if IP is banned
	if h.bruteForce.IsBanned(ip) {
		log.Printf("Banned IP attempted login: %s", ip)
		writeJSON(w, http.StatusForbidden, LoginResponse{
			Success: false,
			Error:   "IP address is temporarily banned",
		})
		return
	}

	// Parse request
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, LoginResponse{
			Success: false,
			Error:   "Invalid request",
		})
		return
	}

	// Validate credentials
	if req.Login != h.login || req.Password != h.password {
		// Record failed attempt
		h.bruteForce.RecordFailure(ip)
		log.Printf("Failed login attempt from %s", ip)

		// Log to fail2ban log file
		h.logToFail2ban(fmt.Sprintf("FAILED LOGIN from %s", ip))

		writeJSON(w, http.StatusUnauthorized, LoginResponse{
			Success: false,
			Error:   "Invalid credentials",
		})
		return
	}

	// Reset failed attempts on successful login
	h.bruteForce.ResetFailures(ip)

	// Generate session token
	token, err := generateToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, LoginResponse{
			Success: false,
			Error:   "Failed to generate token",
		})
		return
	}

	// Create session (no expiry — expires only on logout or tab close)
	session := &Session{
		Token:     token,
		User:      req.Login,
		CreatedAt: time.Now(),
	}

	h.mu.Lock()
	h.sessions[token] = session
	h.mu.Unlock()

	log.Printf("Successful login from %s", ip)

	writeJSON(w, http.StatusOK, LoginResponse{
		Success: true,
		Token:   token,
	})
}

// Logout handles user logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := r.Header.Get("Authorization")
	if token != "" {
		h.mu.Lock()
		delete(h.sessions, token)
		h.mu.Unlock()
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ValidateToken checks if a token is valid
func (h *AuthHandler) ValidateToken(token string) bool {
	h.mu.RLock()
	_, exists := h.sessions[token]
	h.mu.RUnlock()
	return exists
}

// generateToken generates a random token
func generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Use RemoteAddr
	ip := r.RemoteAddr
	if ip[:1] == "[" {
		// IPv6
		if idx := len(ip) - 1; ip[idx] == ']' {
			return ip[1:idx]
		}
	}
	if idx := len(ip) - 1; idx > 0 && ip[idx] == ':' {
		return ip[:idx]
	}
	return ip
}

// writeJSON writes JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// logToFail2ban logs to fail2ban log file
func (h *AuthHandler) logToFail2ban(message string) {
	f, err := os.OpenFile(h.fail2banLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Failed to open fail2ban log: %v", err)
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("%s %s\n", timestamp, message)
	if _, err := f.WriteString(logLine); err != nil {
		log.Printf("Failed to write to fail2ban log: %v", err)
	}
}
