package main

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// authed wraps a handler with static-token auth.
// Accepts Authorization: Bearer <token> or ?token= (EventSource can't set headers).
func authed(cfg *Config, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.URL.Query().Get("token")
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			tok = strings.TrimPrefix(h, "Bearer ")
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(cfg.AuthToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func registerAPI(mux *http.ServeMux, cfg *Config, rc *RconClient, hub *Hub) {
	mux.HandleFunc("POST /api/rcon", authed(cfg, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Command string `json:"command"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Command) == "" {
			http.Error(w, `bad request: expected {"command": "..."}`, http.StatusBadRequest)
			return
		}
		out, err := rc.Exec(req.Command)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "output": out})
	}))

	mux.HandleFunc("GET /api/state", authed(cfg, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, hub.StateSnapshot())
	}))

	mux.HandleFunc("GET /events", authed(cfg, hub.ServeSSE))
}
