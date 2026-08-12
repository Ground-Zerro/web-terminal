package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type Response struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

func fail(w http.ResponseWriter, status int, format string, args ...any) {
	writeJSON(w, status, Response{Error: fmt.Sprintf(format, args...)})
}

func only(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			fail(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		next(w, r)
	}
}

func IndexHandler(page []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write(page); err != nil {
			log.Printf("Failed to write index page: %v", err)
		}
	}
}
