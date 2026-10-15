// Package web serves the proxy dashboard and its JSON API.
package web

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"time"

	"blue-proxy/internal/state"
)

//go:embed index.html
var index []byte

type server struct {
	store *state.Store
	title string
}

// Handler returns the HTTP handler for the proxy dashboard.
func Handler(store *state.Store, title string) http.Handler {
	s := &server{store: store, title: title}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	})
	mux.HandleFunc("/api/state", s.getState)
	mux.HandleFunc("/api/hold", s.postHold)
	mux.HandleFunc("/api/block", s.postBlock)
	return mux
}

func (s *server) getState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"title":      s.title,
		"time":       time.Now(),
		"points":     s.store.Snapshot(),
		"categories": s.store.Categories(),
	})
}

type modeReq struct {
	Key      string `json:"key"`
	Category string `json:"category"` // or "all"
	On       bool   `json:"on"`
}

func (s *server) postHold(w http.ResponseWriter, r *http.Request) {
	s.setMode(w, r, s.store.SetHold, s.store.SetHoldCategory)
}

func (s *server) postBlock(w http.ResponseWriter, r *http.Request) {
	s.setMode(w, r, s.store.SetBlock, s.store.SetBlockCategory)
}

func (s *server) setMode(w http.ResponseWriter, r *http.Request, one func(string, bool) bool, cat func(string, bool) int) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req modeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	n := 0
	switch {
	case req.Key != "":
		if one(req.Key, req.On) {
			n = 1
		}
	case req.Category != "":
		n = cat(req.Category, req.On)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ange key eller category"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"affected": n})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
