# cs2-watch

Lightweight, HLSW-style **live watch panel** for Counter-Strike 2 servers. A single self-hosted binary: real-time log stream, RCON console and live player table in your browser.

![status](https://img.shields.io/badge/status-MVP-orange)

## Why

Classic HLSW died around 2015 and nothing replaced its *live watch* experience for CS2. Existing panels (css-bans, cs2-dashboard) focus on bans/admin management and depend on CounterStrikeSharp plugins — which break on every CS2 update.

cs2-watch uses **only stable Valve protocols**:

| Protocol | Used for |
|---|---|
| `logaddress_add_http` | Real-time log ingestion (native in CS2; the server POSTs log batches and even sends map/game-state/scores as HTTP headers) |
| RCON (TCP) | Console commands from the panel |
| A2S (UDP) | Player list polling |

No plugins to install on the game server. No database. One binary.

## Features

- 📜 **Live log stream** — kills, chat, connects, colored, <1s latency, via SSE
- ⌨️ **RCON console** — command input with output history
- 👥 **Player table** — name / score / time, auto-refreshed every 3s
- 📊 **Server header** — map, game phase, CT/T scores (straight from log POST headers)
- 🔁 **Self-registering** — configures `logaddress_add_http` on the game server itself via RCON, and re-registers automatically after a server restart
- 🔒 **Token auth**, binds to localhost by default, RCON password never leaves the panel host

## Quick start

1. Build (or grab a release binary):

   ```sh
   cd web && pnpm install && pnpm build && cd ..
   go build -o cs2-watch.exe .
   ```

2. Create `config.json` (see `config.example.json`):

   ```json
   {
     "game_server": "192.168.1.50:27015",
     "rcon_password": "yourpassword",
     "listen": "127.0.0.1:8080",
     "ingest_url": "http://127.0.0.1:8080/ingest",
     "auth_token": "a-long-random-string"
   }
   ```

   > `game_server` must be the machine's LAN IP — CS2 binds RCON on it, **not** on 127.0.0.1.
   > `ingest_url` is this panel's `/ingest` endpoint *as reachable from the game server*.

3. Run and open:

   ```sh
   ./cs2-watch.exe          # or: cs2-watch.exe -config path/to/config.json
   # → http://127.0.0.1:8080  (paste your auth_token once)
   ```

## Config reference

| Key | Meaning | Default |
|---|---|---|
| `game_server` | CS2 server `host:port` (LAN IP) | — required |
| `rcon_password` | RCON password | — required |
| `auth_token` | Static token for the panel UI/API | — required |
| `listen` | Panel bind address | `127.0.0.1:8080` |
| `ingest_url` | URL the game server POSTs logs to | — |

## Security model

- The panel binds to `127.0.0.1` by default; opt into LAN with `"listen": "0.0.0.0:8080"`.
- Every UI/API request needs the `auth_token` (Bearer header, or `?token=` for the SSE stream).
- `/ingest` accepts POSTs only from the configured game server IP (or loopback).
- RCON is never exposed — the browser only ever talks to this panel.

## Development

```sh
go run . &                          # backend on :8080
cd web && pnpm dev                  # SvelteKit dev server, proxies /api + /events to :8080
```

Real log samples captured from a live CS2 server (including the `X-Game-*` header contract) live in `testdata/log-samples.txt`.

## Roadmap

- Structured event parsing (kills → frags feed, chat tab)
- Log history + search (SQLite)
- Multi-server support

## License

MIT
