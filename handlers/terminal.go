package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// TerminalHandler handles WebSocket terminal connections
type TerminalHandler struct {
	upgrader     websocket.Upgrader
	terminalDir  string
}

// TerminalMessage represents a message sent over WebSocket
type TerminalMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// TerminalSize represents terminal dimensions
type TerminalSize struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// NewTerminalHandler creates a new terminal handler
func NewTerminalHandler(terminalDir string) *TerminalHandler {
	return &TerminalHandler{
		terminalDir: terminalDir,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  16 * 1024,
			WriteBufferSize: 16 * 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

// HandleWebSocket handles WebSocket connections for terminal
func (h *TerminalHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Validate token
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Set up environment for proper terminal
	env := os.Environ()
	env = append(env,
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"LC_CTYPE=C.UTF-8",
		"TERM_PROGRAM=",
	)

	// Create PTY with proper settings
	cmd := exec.Command("bash", "--login")
	cmd.Dir = h.terminalDir
	cmd.Env = env

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: 80,
		Rows: 24,
	})
	if err != nil {
		log.Printf("Failed to start PTY: %v", err)
		return
	}
	defer ptmx.Close()

	var mu sync.Mutex

	// Read from PTY and send to WebSocket
	go func() {
		buf := make([]byte, 64*1024) // 64KB buffer
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("PTY read error: %v", err)
				}
				return
			}

			if n > 0 {
				mu.Lock()
				err = conn.WriteMessage(websocket.BinaryMessage, buf[:n])
				mu.Unlock()

				if err != nil {
					log.Printf("WebSocket write error: %v", err)
					return
				}
			}
		}
	}()

	// Server-side ping goroutine to keep connection alive through proxies
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				mu.Unlock()
				log.Printf("WebSocket ping error: %v", err)
				return
			}
			mu.Unlock()
		}
	}()

	// Read from WebSocket and write to PTY
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("WebSocket closed: %v", err)
			} else {
				log.Printf("WebSocket read error: %v", err)
			}
			return
		}

		// Handle text messages (JSON commands)
		if messageType == websocket.TextMessage {
			text := string(message)

			// Keepalive ping from client
			if text == "ping" {
				mu.Lock()
				conn.WriteMessage(websocket.TextMessage, []byte("pong"))
				mu.Unlock()
				continue
			}

			var msg TerminalMessage
			if err := json.Unmarshal(message, &msg); err == nil {
				switch msg.Type {
				case "resize":
					var size TerminalSize
					if data, err := json.Marshal(msg.Data); err == nil {
						if err := json.Unmarshal(data, &size); err == nil {
							mu.Lock()
							pty.Setsize(ptmx, &pty.Winsize{
								Cols: uint16(size.Cols),
								Rows: uint16(size.Rows),
							})
							mu.Unlock()
						}
					}
					continue
				}
			}
		}

		// Write binary data directly to PTY
		if _, err := ptmx.Write(message); err != nil {
			log.Printf("PTY write error: %v", err)
			return
		}
	}
}

// IndexHandler serves the main page
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "static/index.html")
}
