package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
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

func registerAPI(mux *http.ServeMux, cfg *Config, rc *RconClient, hub *Hub, bans *BanList) {
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

	mux.HandleFunc("GET /api/meta", authed(cfg, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"quick_commands": cfg.QuickCommands, "maps": cfg.Maps})
	}))

	mux.HandleFunc("GET /api/bans", authed(cfg, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, bans.List())
	}))

	mux.HandleFunc("POST /api/bans", authed(cfg, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SteamID string `json:"steamid"`
			Name    string `json:"name"`
			Reason  string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		req.SteamID = strings.TrimSpace(req.SteamID)
		if req.SteamID == "" || req.SteamID == "BOT" {
			http.Error(w, "steamid required (bots can't be banned)", http.StatusBadRequest)
			return
		}
		entry := BanEntry{SteamID: req.SteamID, Name: req.Name, Reason: req.Reason, CreatedAt: time.Now()}
		if p, ok := hub.FindBySteamID(req.SteamID); ok { // online: capture identity and kick now
			if entry.Name == "" {
				entry.Name = p.Name
			}
			entry.IP = p.Addr
			go rc.Exec(fmt.Sprintf("kickid %d", p.UserID))
		}
		bans.Add(entry)
		writeJSON(w, map[string]any{"ok": true})
	}))

	mux.HandleFunc("POST /api/unban", authed(cfg, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SteamID string `json:"steamid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": bans.Remove(strings.TrimSpace(req.SteamID))})
	}))

	mux.HandleFunc("GET /events", authed(cfg, hub.ServeSSE))
}
