package main

import (
	"log"
	"reflect"
	"time"

	a2s "github.com/rumblefrog/go-a2s"
)

// a2sLoop polls the game server for the player list every 3s and pushes
// changes to the hub. A2S is a plain Valve UDP query — no plugin dependency.
func a2sLoop(cfg *Config, hub *Hub) {
	var last []Player
	errLogged := false
	for ; ; time.Sleep(3 * time.Second) {
		players, err := queryPlayers(cfg.GameServer)
		if err != nil {
			if !errLogged { // log once per outage, not every 3s
				log.Printf("a2s: %v (retrying quietly)", err)
				errLogged = true
			}
			continue
		}
		errLogged = false
		if reflect.DeepEqual(players, last) {
			continue
		}
		last = players
		hub.SetPlayers(players)
	}
}

func queryPlayers(addr string) ([]Player, error) {
	client, err := a2s.NewClient(addr)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	info, err := client.QueryPlayer()
	if err != nil {
		return nil, err
	}
	players := make([]Player, 0, len(info.Players))
	for _, p := range info.Players {
		players = append(players, Player{Name: p.Name, Score: int(p.Score), Duration: p.Duration})
	}
	return players, nil
}
