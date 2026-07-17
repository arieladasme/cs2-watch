package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// State is the last known snapshot of the game server, fed by the X-Game-*
// headers of logaddress_add_http POSTs and the A2S poller.
type State struct {
	Map       string    `json:"map"`
	GameState string    `json:"game_state"`
	ScoreCT   string    `json:"score_ct"`
	ScoreT    string    `json:"score_t"`
	Players   []Player  `json:"players"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Player struct {
	Name     string  `json:"name"`
	Score    int     `json:"score"`
	Duration float32 `json:"duration_s"`
}

type event struct {
	Type string `json:"type"` // snapshot | lines | state | players
	Data any    `json:"data"`
}

// Hub keeps the log ring buffer and latest state, and fans events out to SSE clients.
type Hub struct {
	mu            sync.Mutex
	lines         []string
	max           int
	state         State
	instanceToken string
	lastPost      time.Time
	clients       map[chan []byte]struct{}
}

func NewHub(maxLines int) *Hub {
	return &Hub{max: maxLines, clients: make(map[chan []byte]struct{})}
}

func (h *Hub) broadcastLocked(ev event) {
	msg, err := json.Marshal(ev)
	if err != nil {
		return
	}
	for ch := range h.clients {
		select {
		case ch <- msg:
		default: // ponytail: a slow client misses events; dropping beats blocking the hub
		}
	}
}

// AddLines appends log lines to the ring buffer and broadcasts them.
func (h *Hub) AddLines(lines []string) {
	if len(lines) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lines = append(h.lines, lines...)
	if len(h.lines) > h.max*2 { // amortized trim: copy once the slice doubles past max
		h.lines = append([]string(nil), h.lines[len(h.lines)-h.max:]...)
	}
	h.lastPost = time.Now()
	h.broadcastLocked(event{Type: "lines", Data: lines})
}

// SetGameState updates header-derived state; reports whether the game server
// instance changed (restart detected via X-Server-Instance-Token).
func (h *Hub) SetGameState(mapName, gameState, scoreCT, scoreT, token string) (restarted bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	restarted = h.instanceToken != "" && token != "" && token != h.instanceToken
	h.instanceToken = token
	h.state.Map, h.state.GameState = mapName, gameState
	h.state.ScoreCT, h.state.ScoreT = scoreCT, scoreT
	h.state.UpdatedAt = time.Now()
	h.lastPost = time.Now()
	h.broadcastLocked(event{Type: "state", Data: h.state})
	return restarted
}

func (h *Hub) SetPlayers(players []Player) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.state.Players = players
	h.state.UpdatedAt = time.Now()
	h.broadcastLocked(event{Type: "players", Data: players})
}

func (h *Hub) StateSnapshot() State {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}

// LastPost reports when the game server last POSTed logs (for re-registration).
func (h *Hub) LastPost() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastPost
}

func (h *Hub) tailLocked(n int) []string {
	if len(h.lines) < n {
		n = len(h.lines)
	}
	return append([]string(nil), h.lines[len(h.lines)-n:]...)
}

// ServeSSE streams events to one client: a snapshot first, then live events.
// Heartbeat comments every 15s keep proxies from closing the connection.
func (h *Hub) ServeSSE(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	ch := make(chan []byte, 256)
	h.mu.Lock()
	snap, _ := json.Marshal(event{Type: "snapshot", Data: map[string]any{
		"state": h.state,
		"lines": h.tailLocked(200),
	}})
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}()

	fmt.Fprintf(w, "data: %s\n\n", snap)
	fl.Flush()

	hb := time.NewTicker(15 * time.Second)
	defer hb.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			fl.Flush()
		case <-hb.C:
			fmt.Fprint(w, ": hb\n\n")
			fl.Flush()
		}
	}
}
