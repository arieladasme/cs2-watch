package main

import (
	"bufio"
	"log"
	"net"
	"net/http"
	"strings"
)

// registerIngest wires POST /ingest, the endpoint the game server is pointed
// at via logaddress_add_http. The game server can't send auth, so the check is
// remote IP == configured game server host (or loopback for same-machine setups).
func registerIngest(mux *http.ServeMux, cfg *Config, hub *Hub) {
	gameHost, _, _ := net.SplitHostPort(cfg.GameServer)

	mux.HandleFunc("POST /ingest", func(w http.ResponseWriter, r *http.Request) {
		remote, _, _ := net.SplitHostPort(r.RemoteAddr)
		if remote != gameHost {
			ip := net.ParseIP(remote)
			if ip == nil || !ip.IsLoopback() {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		// Headers carry a free state snapshot per POST (validated 2026-07-17,
		// see testdata/log-samples.txt): map, game state, team scores, instance token.
		restarted := hub.SetGameState(
			r.Header.Get("X-Game-Map"),
			r.Header.Get("X-Game-State"),
			r.Header.Get("X-Game-ScoreCT"),
			r.Header.Get("X-Game-ScoreT"),
			r.Header.Get("X-Server-Instance-Token"),
		)
		if restarted {
			log.Printf("game server instance changed (restart detected)")
		}

		var lines []string
		sc := bufio.NewScanner(r.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			if line := strings.TrimRight(sc.Text(), "\r"); line != "" {
				lines = append(lines, line)
			}
		}
		hub.ApplyLogLines(lines)
		hub.AddLines(lines)

		// 200 acks this X-LogBytes range; the server then advances its buffer.
		w.WriteHeader(http.StatusOK)
	})
}
