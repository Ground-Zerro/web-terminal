package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

const (
	wsBufferSize  = 16 << 10
	ptyBufferSize = 64 << 10
	pingPeriod    = 30 * time.Second
	writeWait     = 10 * time.Second
	defaultCols   = 80
	defaultRows   = 24
)

var (
	pingFrame = []byte("ping")
	pongFrame = []byte("pong")

	shellEnv = []string{
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"LC_CTYPE=C.UTF-8",
		"TERM_PROGRAM=",
	}
)

type TerminalHandler struct {
	upgrader    websocket.Upgrader
	terminalDir string
}

type terminalMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type terminalSize struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func NewTerminalHandler(terminalDir string) *TerminalHandler {
	return &TerminalHandler{
		terminalDir: terminalDir,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  wsBufferSize,
			WriteBufferSize: wsBufferSize,
			CheckOrigin:     sameOrigin,
		},
	}
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func (h *TerminalHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	session, err := h.startShell()
	if err != nil {
		log.Printf("Failed to start PTY: %v", err)
		return
	}
	defer session.close()

	done := make(chan struct{})
	defer close(done)

	go session.pumpOutput(conn)
	go session.keepAlive(conn, done)

	session.pumpInput(conn)
}

type shellSession struct {
	cmd  *exec.Cmd
	ptmx *os.File

	writeMu sync.Mutex
}

func (h *TerminalHandler) startShell() (*shellSession, error) {
	cmd := exec.Command("bash", "--login")
	cmd.Dir = h.terminalDir
	cmd.Env = append(os.Environ(), shellEnv...)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: defaultCols, Rows: defaultRows})
	if err != nil {
		return nil, err
	}
	return &shellSession{cmd: cmd, ptmx: ptmx}, nil
}

func (s *shellSession) close() {
	s.ptmx.Close()
	if s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	s.cmd.Wait()
}

func (s *shellSession) write(conn *websocket.Conn, messageType int, payload []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return conn.WriteMessage(messageType, payload)
}

func (s *shellSession) pumpOutput(conn *websocket.Conn) {
	buf := make([]byte, ptyBufferSize)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			if err := s.write(conn, websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				log.Printf("PTY read error: %v", err)
			}
			conn.Close()
			return
		}
	}
}

func (s *shellSession) keepAlive(conn *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := s.write(conn, websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *shellSession) pumpInput(conn *websocket.Conn) {
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("WebSocket read error: %v", err)
			}
			return
		}

		if messageType == websocket.TextMessage && s.handleControl(conn, message) {
			continue
		}

		if _, err := s.ptmx.Write(message); err != nil {
			log.Printf("PTY write error: %v", err)
			return
		}
	}
}

func (s *shellSession) handleControl(conn *websocket.Conn, message []byte) bool {
	if bytes.Equal(message, pingFrame) {
		s.write(conn, websocket.TextMessage, pongFrame)
		return true
	}

	if len(message) == 0 || message[0] != '{' {
		return false
	}

	var msg terminalMessage
	if err := json.Unmarshal(message, &msg); err != nil || msg.Type != "resize" {
		return false
	}

	var size terminalSize
	if err := json.Unmarshal(msg.Data, &size); err != nil || size.Cols == 0 || size.Rows == 0 {
		return true
	}

	if err := pty.Setsize(s.ptmx, &pty.Winsize{Cols: size.Cols, Rows: size.Rows}); err != nil {
		log.Printf("PTY resize error: %v", err)
	}
	return true
}
