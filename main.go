// cs2-watch: lightweight live-watch panel for CS2 servers (HLSW-style).
// Single binary; talks to the game server via RCON, A2S and logaddress_add_http.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type Config struct {
	GameServer   string `json:"game_server"`   // host:port — use the LAN IP (CS2 binds RCON on it, not loopback)
	RconPassword string `json:"rcon_password"` //
	Listen       string `json:"listen"`        // panel bind address, default 127.0.0.1:8080
	IngestURL    string `json:"ingest_url"`    // URL the game server POSTs logs to (this panel's /ingest)
	AuthToken    string `json:"auth_token"`    // static token for the panel API/SSE
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.GameServer == "" || cfg.RconPassword == "" || cfg.AuthToken == "" {
		return nil, fmt.Errorf("%s: game_server, rcon_password and auth_token are required", path)
	}
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:8080"
	}
	return &cfg, nil
}

func main() {
	cfgPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	rc := NewRconClient(cfg.GameServer, cfg.RconPassword)
	hub := NewHub(5000)

	mux := http.NewServeMux()
	registerAPI(mux, cfg, rc, hub)
	registerIngest(mux, cfg, hub)
	registerWeb(mux)
	go logAddressLoop(cfg, rc, hub)
	go a2sLoop(cfg, hub)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("cs2-watch listening on http://%s (game server %s)", cfg.Listen, cfg.GameServer)
	log.Fatal(srv.ListenAndServe())
}

// logAddressLoop registers our /ingest endpoint on the game server and
// re-registers whenever POSTs stop for 60s (a server restart loses the
// registration). Re-adding the same URL is deduped server-side; an idle empty
// server just gets a harmless periodic re-add. Verified against a live server
// in the end-to-end check.
func logAddressLoop(cfg *Config, rc *RconClient, hub *Hub) {
	register := func() {
		if _, err := rc.Exec("log on"); err != nil {
			log.Printf("rcon: log on failed: %v (will retry)", err)
			return
		}
		if _, err := rc.Exec(fmt.Sprintf("logaddress_add_http %q", cfg.IngestURL)); err != nil {
			log.Printf("rcon: logaddress_add_http failed: %v", err)
			return
		}
		log.Printf("log ingestion registered at %s", cfg.IngestURL)
	}
	register()
	for range time.Tick(30 * time.Second) {
		if time.Since(hub.LastPost()) > 60*time.Second {
			register()
		}
	}
}
