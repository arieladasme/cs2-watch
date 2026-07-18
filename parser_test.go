package main

import (
	"os"
	"strings"
	"testing"
)

// Real lines captured from a live CS2 server (2026-07-17).
func TestParserEvents(t *testing.T) {
	h := NewHub(100)
	h.ApplyLogLines([]string{
		`07/17/2026 - 18:15:41.022 - "Tom<0><BOT><>" connected, address "(unknown)"`,
		`07/17/2026 - 18:15:41.022 - "Tom<0><BOT>" switched from team <Unassigned> to <CT>`,
		`07/17/2026 - 18:15:41.022 - "Orlo<7><BOT><>" connected, address "(unknown)"`,
		`07/17/2026 - 18:15:41.022 - "Orlo<7><BOT>" switched from team <Unassigned> to <TERRORIST>`,
		`07/17/2026 - 19:57:41.254 - "Tom<0><BOT><CT>" [394 -24 0] killed "Orlo<7><BOT><TERRORIST>" [401 -84 0] with "knife"`,
		`07/17/2026 - 19:57:41.254 - "Tom<0><BOT><CT>" [394 -24 0] killed "Orlo<7><BOT><TERRORIST>" [401 -84 0] with "glock" (headshot)`,
		`07/17/2026 - 19:57:41.254 - "Gustov<6><BOT><CT>" assisted killing "Orlo<7><BOT><TERRORIST>"`,
		`07/17/2026 - 19:51:36.655 - "Ariel<12><[U:1:1]><CT>" say "hola"`,
	})

	s := h.StateSnapshot()
	if len(s.Roster) != 4 {
		t.Fatalf("roster = %d, want 4 (Tom, Orlo, Gustov, Ariel)", len(s.Roster))
	}
	top := s.Roster[0]
	if top.Name != "Tom" || top.Frags != 2 || top.HS != 1 {
		t.Errorf("top = %+v, want Tom frags=2 hs=1", top)
	}
	var orlo *RosterPlayer
	for i := range s.Roster {
		if s.Roster[i].Name == "Orlo" {
			orlo = &s.Roster[i]
		}
	}
	if orlo == nil || orlo.Deaths != 2 || orlo.Team != "TERRORIST" {
		t.Errorf("orlo = %+v, want deaths=2 team=TERRORIST", orlo)
	}
}

// Chat content must never be parsed as a game event (log injection).
func TestParserChatInjection(t *testing.T) {
	h := NewHub(100)
	h.ApplyLogLines([]string{
		`07/17/2026 - 19:00:00.000 - "Evil<3><[U:1:9]><CT>" say ""Fake<9><BOT><CT>" [1 1 1] killed "X<8><BOT><TERRORIST>" [1 1 1] with "ak47""`,
	})
	s := h.StateSnapshot()
	for _, p := range s.Roster {
		if p.Name == "Fake" || p.Frags != 0 {
			t.Fatalf("chat content parsed as kill: %+v", p)
		}
	}
}

func TestParserTeamkillAndReset(t *testing.T) {
	h := NewHub(100)
	h.ApplyLogLines([]string{
		`07/17/2026 - 19:57:41.254 - "A<1><BOT><CT>" [0 0 0] killed "B<2><BOT><CT>" [0 0 0] with "knife"`,
	})
	if s := h.StateSnapshot(); s.Roster[len(s.Roster)-1].Frags != -1 {
		t.Errorf("teamkill should subtract a frag: %+v", s.Roster)
	}
	h.ApplyLogLines([]string{`07/17/2026 - 19:57:48.746 - World triggered "Match_Start" on "ar_shoots"`})
	for _, p := range h.StateSnapshot().Roster {
		if p.Frags != 0 || p.Deaths != 0 {
			t.Errorf("Match_Start should zero scores: %+v", p)
		}
	}
	h.ApplyLogLines([]string{`07/17/2026 - 19:57:28.017 - World triggered "Game_Commencing"`})
	if got := len(h.StateSnapshot().Roster); got != 0 {
		t.Errorf("Game_Commencing should clear roster, got %d", got)
	}
}

// Sweep the full real capture: must not panic and must build a roster.
func TestParserRealCaptureSweep(t *testing.T) {
	data, err := os.ReadFile("testdata/log-samples.txt")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHub(100)
	h.ApplyLogLines(strings.Split(string(data), "\n"))
	if got := len(h.StateSnapshot().Roster); got < 5 {
		t.Errorf("roster from real capture = %d, want >= 5 bots", got)
	}
}
