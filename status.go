package main

import (
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// statusLoop polls `status` over RCON every 10s: it is the only source of
// ping and the authoritative online list (used to prune stale roster rows).
// Real row formats:
//
//	   0      BOT    0    0     active      0 'Shaur'
//	  12    05:32   23    0     active 786432 192.168.1.50:27005 'Ariel'
var reStatusRow = regexp.MustCompile(`^\s*(\d+)\s+(\S+)\s+(\d+)\s+(\d+)\s+(\S+)\s+(\d+)\s+(?:(\S+)\s+)?'(.*)'\s*$`)

type netInfo struct {
	Name string
	Ping int
	Addr string
	Bot  bool
}

func parseStatus(out string) map[int]netInfo {
	inPlayers := false
	infos := make(map[int]netInfo)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "---------players"):
			inPlayers = true
			continue
		case strings.HasPrefix(line, "#end"):
			return infos
		}
		if !inPlayers {
			continue
		}
		m := reStatusRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id, _ := strconv.Atoi(m[1])
		ping, _ := strconv.Atoi(m[3])
		infos[id] = netInfo{Name: m[8], Ping: ping, Addr: m[7], Bot: m[2] == "BOT"}
	}
	if !inPlayers {
		return nil // unrecognized output; don't wipe the roster with it
	}
	return infos
}

func statusLoop(rc *RconClient, hub *Hub) {
	errLogged := false
	for ; ; time.Sleep(10 * time.Second) {
		out, err := rc.Exec("status")
		if err != nil {
			if !errLogged {
				log.Printf("status poll: %v (retrying quietly)", err)
				errLogged = true
			}
			continue
		}
		errLogged = false
		if infos := parseStatus(out); infos != nil {
			hub.SetNetInfo(infos)
		}
	}
}

// SetNetInfo merges status data into the roster: ping/addr/name updates,
// self-heals players we never saw connect, and prunes ghosts that left.
func (h *Hub) SetNetInfo(infos map[int]netInfo) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, si := range infos {
		p, ok := h.roster[id]
		if !ok {
			p = &RosterPlayer{UserID: id}
			h.roster[id] = p
		}
		p.Name = si.Name
		p.Ping = si.Ping
		if si.Bot {
			p.Bot = true
			p.SteamID = "BOT"
		}
		if si.Addr != "" {
			p.Addr = si.Addr
		}
	}
	for id := range h.roster {
		if _, ok := infos[id]; !ok {
			delete(h.roster, id)
		}
	}
	h.state.Roster = h.rosterSliceLocked()
	h.broadcastLocked(event{Type: "roster", Data: h.state.Roster})
}
