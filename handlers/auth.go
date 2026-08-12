package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"webterminal/middleware"
)

const (
	tokenBytes      = 32
	sessionSweep    = 10 * time.Minute
	fail2banLogPerm = 0o640
)

type Credentials struct {
	Login       string
	Password    string
	Fail2banLog string
	SessionTTL  time.Duration
}

type AuthHandler struct {
	bruteForce  *middleware.BruteForce
	loginHash   [sha256.Size]byte
	passHash    [sha256.Size]byte
	fail2banLog string
	ttl         time.Duration

	mu       sync.Mutex
	sessions map[string]time.Time
}

func NewAuthHandler(bruteForce *middleware.BruteForce, creds Credentials) *AuthHandler {
	h := &AuthHandler{
		bruteForce:  bruteForce,
		loginHash:   sha256.Sum256([]byte(creds.Login)),
		passHash:    sha256.Sum256([]byte(creds.Password)),
		fail2banLog: creds.Fail2banLog,
		ttl:         creds.SessionTTL,
		sessions:    make(map[string]time.Time),
	}
	go h.sweep()
	return h
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type loginResponse struct {
	Response
	Token string `json:"token,omitempty"`
	User  string `json:"user,omitempty"`
}

func (h *AuthHandler) Require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.validate(requestToken(r)) {
			fail(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		next(w, r)
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	only(http.MethodPost, h.login)(w, r)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	only(http.MethodPost, h.logout)(w, r)
}

func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)

	if h.bruteForce.IsBanned(ip) {
		log.Printf("Banned IP attempted login: %s", ip)
		fail(w, http.StatusForbidden, "IP address is temporarily banned")
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if !h.matches(req) {
		h.bruteForce.RecordFailure(ip)
		log.Printf("Failed login attempt from %s", ip)
		h.logToFail2ban(fmt.Sprintf("FAILED LOGIN from %s", ip))
		fail(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	h.bruteForce.ResetFailures(ip)

	token, err := generateToken()
	if err != nil {
		log.Printf("Failed to generate token: %v", err)
		fail(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	h.mu.Lock()
	h.sessions[token] = time.Now().Add(h.ttl)
	h.mu.Unlock()

	log.Printf("Successful login from %s", ip)
	writeJSON(w, http.StatusOK, loginResponse{
		Response: Response{Success: true},
		Token:    token,
		User:     req.Login,
	})
}

func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	if token := requestToken(r); token != "" {
		h.mu.Lock()
		delete(h.sessions, token)
		h.mu.Unlock()
	}
	writeJSON(w, http.StatusOK, Response{Success: true})
}

func (h *AuthHandler) matches(req loginRequest) bool {
	login := sha256.Sum256([]byte(req.Login))
	pass := sha256.Sum256([]byte(req.Password))
	sameLogin := subtle.ConstantTimeCompare(login[:], h.loginHash[:])
	samePass := subtle.ConstantTimeCompare(pass[:], h.passHash[:])
	return sameLogin&samePass == 1
}

func (h *AuthHandler) validate(token string) bool {
	if token == "" {
		return false
	}

	now := time.Now()

	h.mu.Lock()
	defer h.mu.Unlock()

	expiry, exists := h.sessions[token]
	if !exists {
		return false
	}
	if now.After(expiry) {
		delete(h.sessions, token)
		return false
	}

	h.sessions[token] = now.Add(h.ttl)
	return true
}

func (h *AuthHandler) sweep() {
	ticker := time.NewTicker(sessionSweep)
	defer ticker.Stop()

	for now := range ticker.C {
		h.mu.Lock()
		for token, expiry := range h.sessions {
			if now.After(expiry) {
				delete(h.sessions, token)
			}
		}
		h.mu.Unlock()
	}
}

func (h *AuthHandler) logToFail2ban(message string) {
	if h.fail2banLog == "" {
		return
	}

	f, err := os.OpenFile(h.fail2banLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fail2banLogPerm)
	if err != nil {
		log.Printf("Failed to open fail2ban log: %v", err)
		return
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.DateTime), message); err != nil {
		log.Printf("Failed to write to fail2ban log: %v", err)
	}
}

func requestToken(r *http.Request) string {
	if token := r.Header.Get("Authorization"); token != "" {
		return token
	}
	return r.URL.Query().Get("token")
}

func generateToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return host
	}

	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		return real
	}

	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if i := strings.LastIndexByte(forwarded, ','); i >= 0 {
			forwarded = forwarded[i+1:]
		}
		if last := strings.TrimSpace(forwarded); last != "" {
			return last
		}
	}

	return host
}
