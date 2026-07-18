package main

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"sort"
	"sync"
	"time"
)

// CS2 dropped the old banid/listid system, so cs2-watch enforces its own ban
// list: entries persist in a JSON file and banned players are kicked the
// moment their connect line arrives (steamid or IP match).
// ponytail: enforcement needs the panel online at connect time; a player who
// connected while the panel was down stays until next map change.
type BanEntry struct {
	SteamID   string    `json:"steamid"`
	IP        string    `json:"ip,omitempty"`
	Name      string    `json:"name"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type BanList struct {
	mu      sync.Mutex
	path    string
	entries map[string]BanEntry // keyed by steamid
}

func LoadBans(path string) *BanList {
	b := &BanList{path: path, entries: make(map[string]BanEntry)}
	data, err := os.ReadFile(path)
	if err != nil {
		return b // missing file = empty list
	}
	var list []BanEntry
	if err := json.Unmarshal(data, &list); err != nil {
		log.Printf("bans: cannot parse %s: %v (starting empty)", path, err)
		return b
	}
	for _, e := range list {
		b.entries[e.SteamID] = e
	}
	return b
}

func (b *BanList) saveLocked() {
	data, err := json.MarshalIndent(b.listLocked(), "", "  ")
	if err == nil {
		err = os.WriteFile(b.path, data, 0o644)
	}
	if err != nil {
		log.Printf("bans: cannot save %s: %v", b.path, err)
	}
}

func (b *BanList) listLocked() []BanEntry {
	out := make([]BanEntry, 0, len(b.entries))
	for _, e := range b.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (b *BanList) List() []BanEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.listLocked()
}

func (b *BanList) Add(e BanEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries[e.SteamID] = e
	b.saveLocked()
}

func (b *BanList) Remove(steamID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.entries[steamID]; !ok {
		return false
	}
	delete(b.entries, steamID)
	b.saveLocked()
	return true
}

func hostOf(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}

// Match reports whether a steamid or address is banned.
func (b *BanList) Match(steamID, addr string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if steamID != "" && steamID != "BOT" {
		if _, ok := b.entries[steamID]; ok {
			return true
		}
	}
	host := hostOf(addr)
	if host == "" || host == "(unknown)" {
		return false
	}
	for _, e := range b.entries {
		if e.IP != "" && hostOf(e.IP) == host {
			return true
		}
	}
	return false
}
