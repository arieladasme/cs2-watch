package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBanMatchAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bans.json")
	b := LoadBans(path)
	b.Add(BanEntry{SteamID: "[U:1:111]", IP: "10.0.0.5:27005", Name: "Griefer", CreatedAt: time.Now()})

	if !b.Match("[U:1:111]", "") {
		t.Error("steamid match failed")
	}
	if !b.Match("[U:1:999]", "10.0.0.5:1234") {
		t.Error("IP match should ignore port")
	}
	if b.Match("[U:1:999]", "10.0.0.6:1234") {
		t.Error("unrelated player matched")
	}
	if b.Match("BOT", "") || b.Match("", "") {
		t.Error("BOT/empty steamid must never match")
	}

	// reload from disk
	b2 := LoadBans(path)
	if !b2.Match("[U:1:111]", "") {
		t.Error("ban did not persist")
	}
	if !b2.Remove("[U:1:111]") || b2.Match("[U:1:111]", "") {
		t.Error("unban failed")
	}
}

func TestBannedConnectTriggersKick(t *testing.T) {
	b := LoadBans(filepath.Join(t.TempDir(), "bans.json"))
	b.Add(BanEntry{SteamID: "[U:1:42]", CreatedAt: time.Now()})

	kicked := make(chan int, 1)
	h := NewHub(10)
	h.SetBanEnforcement(b, func(userid int) { kicked <- userid })

	h.ApplyLogLines([]string{
		`07/17/2026 - 20:00:00.000 - "Cheater<7><[U:1:42]><>" connected, address "10.1.2.3:27005"`,
	})
	select {
	case id := <-kicked:
		if id != 7 {
			t.Errorf("kicked userid = %d, want 7", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("banned connect did not trigger kick")
	}
}

func TestParseStatusRows(t *testing.T) {
	out := "hostname : test\n---------players--------\n" +
		"  id     time ping loss      state   rate adr name\n" +
		"   0      BOT    0    0     active      0 'Shaur'\n" +
		"  12    05:32   23    0     active 786432 192.168.1.50:27005 'Ariel'\n" +
		"#end\n"
	infos := parseStatus(out)
	if len(infos) != 2 {
		t.Fatalf("parsed %d rows, want 2", len(infos))
	}
	if !infos[0].Bot || infos[0].Name != "Shaur" {
		t.Errorf("bot row = %+v", infos[0])
	}
	human := infos[12]
	if human.Bot || human.Ping != 23 || human.Addr != "192.168.1.50:27005" || human.Name != "Ariel" {
		t.Errorf("human row = %+v", human)
	}
	if parseStatus("garbage with no players section") != nil {
		t.Error("unrecognized output must return nil, not an empty roster wipe")
	}
}

func TestConnectCapturesAddr(t *testing.T) {
	h := NewHub(10)
	h.ApplyLogLines([]string{
		`07/17/2026 - 20:00:00.000 - "Ariel<12><[U:1:1]><>" connected, address "192.168.1.50:27005"`,
	})
	p, ok := h.FindBySteamID("[U:1:1]")
	if !ok || p.Addr != "192.168.1.50:27005" {
		t.Errorf("addr not captured: %+v", p)
	}
}
