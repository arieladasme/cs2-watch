package main

import (
	"regexp"
	"sort"
	"strconv"
)

// Log line grammar, from real server captures (see testdata/log-samples.txt):
//
//	"Juuvy<1><BOT><CT>" [394 -24 0] killed "Orlo<7><BOT><TERRORIST>" [401 -84 0] with "knife" (headshot)
//	"Gustov<6><BOT><CT>" assisted killing "Orlo<7><BOT><TERRORIST>"
//	"Tom<0><BOT><>" connected, address "(unknown)"
//	"Bluefish<0><BOT>" switched from team <Unassigned> to <CT>
//	"Ariel<12><[U:1:123]><CT>" say "hello"
//	World triggered "Match_Start" on "ar_shoots"
//
// Player token: "NAME<userid><steamid>" with an optional trailing <TEAM>.
// Lines are timestamp-prefixed; the prefix is stripped and patterns are
// anchored at ^ so player-controlled chat content can't spoof events.
const pRe = `"(.*?)<(\d+)><([^>]*)>(?:<([^>]*)>)?"`

var (
	reTS       = regexp.MustCompile(`^\d{2}/\d{2}/\d{4} - \d{2}:\d{2}:\d{2}\.\d{3} - `)
	reSay      = regexp.MustCompile(`^` + pRe + ` say(_team)? "(.*)"$`)
	reKill     = regexp.MustCompile(`^` + pRe + ` \[-?\d+ -?\d+ -?\d+\] killed ` + pRe + ` \[-?\d+ -?\d+ -?\d+\] with "([^"]+)"( \(headshot\))?`)
	reAssist   = regexp.MustCompile(`^` + pRe + ` assisted killing ` + pRe)
	reSuicide  = regexp.MustCompile(`^` + pRe + ` \[-?\d+ -?\d+ -?\d+\] committed suicide with "([^"]+)"`)
	reConnect  = regexp.MustCompile(`^` + pRe + ` connected, address`)
	reDisconn  = regexp.MustCompile(`^` + pRe + ` disconnected`)
	reSwitched = regexp.MustCompile(`^"(.*?)<(\d+)><([^>]*)>" switched from team <([^>]*)> to <([^>]*)>$`)
	reWorld    = regexp.MustCompile(`^World triggered "([^"]+)"(?: on "([^"]+)")?`)
)

type RosterPlayer struct {
	UserID  int    `json:"userid"`
	Name    string `json:"name"`
	SteamID string `json:"steamid"`
	Bot     bool   `json:"bot"`
	Team    string `json:"team"`
	Frags   int    `json:"frags"`
	Deaths  int    `json:"deaths"`
	HS      int    `json:"hs"`
	Assists int    `json:"assists"`
}

type ChatMsg struct {
	Name     string `json:"name"`
	Team     string `json:"team"`
	Msg      string `json:"msg"`
	TeamOnly bool   `json:"team_only"`
}

type parsedPlayer struct {
	name, steamID, team string
	userID              int
}

// player extracts a pRe match starting at group index base (pRe has 4 groups).
func player(m []string, base int) parsedPlayer {
	id, _ := strconv.Atoi(m[base+1])
	return parsedPlayer{name: m[base], userID: id, steamID: m[base+2], team: m[base+3]}
}

// ApplyLogLines runs every line through the event parser, updating the roster
// and chat buffer, and broadcasts one roster event per batch if anything changed.
func (h *Hub) ApplyLogLines(lines []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	changed := false
	for _, raw := range lines {
		if h.applyLocked(reTS.ReplaceAllString(raw, "")) {
			changed = true
		}
	}
	if changed {
		h.state.Roster = h.rosterSliceLocked()
		h.broadcastLocked(event{Type: "roster", Data: h.state.Roster})
	}
}

// applyLocked parses one line; reports whether the roster changed.
// Order matters: say first, so chat content can never reach the other patterns.
func (h *Hub) applyLocked(line string) bool {
	if m := reSay.FindStringSubmatch(line); m != nil {
		p := player(m, 1)
		_, existed := h.roster[p.userID]
		h.ensureLocked(p) // a chatting player is in-game even if we missed their connect
		msg := ChatMsg{Name: p.name, Team: p.team, Msg: m[6], TeamOnly: m[5] != ""}
		h.chat = append(h.chat, msg)
		if len(h.chat) > 100 {
			h.chat = h.chat[len(h.chat)-100:]
		}
		h.broadcastLocked(event{Type: "chat", Data: msg})
		return !existed
	}
	if m := reKill.FindStringSubmatch(line); m != nil {
		k, v := player(m, 1), player(m, 5)
		kp, vp := h.ensureLocked(k), h.ensureLocked(v)
		if k.team != "" && k.team == v.team {
			kp.Frags-- // teamkill
		} else {
			kp.Frags++
		}
		if m[10] != "" {
			kp.HS++
		}
		vp.Deaths++
		return true
	}
	if m := reAssist.FindStringSubmatch(line); m != nil {
		h.ensureLocked(player(m, 1)).Assists++
		return true
	}
	if m := reSuicide.FindStringSubmatch(line); m != nil {
		p := h.ensureLocked(player(m, 1))
		p.Frags--
		p.Deaths++
		return true
	}
	if m := reConnect.FindStringSubmatch(line); m != nil {
		h.ensureLocked(player(m, 1))
		return true
	}
	if m := reDisconn.FindStringSubmatch(line); m != nil {
		delete(h.roster, player(m, 1).userID)
		return true
	}
	if m := reSwitched.FindStringSubmatch(line); m != nil {
		id, _ := strconv.Atoi(m[2])
		p := h.ensureLocked(parsedPlayer{name: m[1], userID: id, steamID: m[3]})
		p.Team = m[5]
		return true
	}
	if m := reWorld.FindStringSubmatch(line); m != nil {
		switch m[1] {
		case "Game_Commencing":
			// map change: userids get reassigned, players re-log their connects
			h.roster = map[int]*RosterPlayer{}
			return true
		case "Match_Start":
			for _, p := range h.roster {
				p.Frags, p.Deaths, p.HS, p.Assists = 0, 0, 0, 0
			}
			return true
		}
	}
	return false
}

// ensureLocked upserts a roster entry by userid, refreshing identity fields.
func (h *Hub) ensureLocked(pp parsedPlayer) *RosterPlayer {
	p, ok := h.roster[pp.userID]
	if !ok {
		p = &RosterPlayer{UserID: pp.userID}
		h.roster[pp.userID] = p
	}
	p.Name = pp.name
	p.SteamID = pp.steamID
	p.Bot = pp.steamID == "BOT"
	if pp.team != "" {
		p.Team = pp.team
	}
	return p
}

func (h *Hub) rosterSliceLocked() []RosterPlayer {
	out := make([]RosterPlayer, 0, len(h.roster))
	for _, p := range h.roster {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Frags != out[j].Frags {
			return out[i].Frags > out[j].Frags
		}
		return out[i].Name < out[j].Name
	})
	return out
}
