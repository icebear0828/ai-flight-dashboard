# ✈️ AI Flight Dashboard

> Minimal, zero-dependency AI cost tracking terminal dashboard

[中文](README.md)

AI Flight Dashboard is a **TUI + Web dual-mode tool** built in Go. Using a **"passive radar"** approach, it non-invasively captures token usage from AI CLI tools (Claude Code, Gemini CLI) by watching their log files in real time, and presents live cost feedback with smooth terminal animations.

## ✨ Features

- 🎯 **Passive Radar**: Uses `fsnotify` to watch incremental file streams — the moment a tool writes logs to disk (`~/.claude/projects/`, `~/.gemini/tmp/`), the dashboard captures them instantly.
- ⚡ **Blazing Fast**: Built with Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea). Single binary, zero runtime dependencies, instant startup.
- 💰 **Real-time Cost Calculation**: Built-in pricing engine converts raw token counts into USD costs per model.
- 💾 **SQLite Persistence**: All captured usage is automatically upserted into `stats/usage.db` for long-term analysis.
- 🌐 **Web Dashboard**: Start an HTTP server with `--web` to view a React-powered visual dashboard.
- 📡 **Multi-device Aggregation**: Supports `--device-id` to identify machines and filter API queries.
- 🛰️ **Remote Telemetry**: Use `--forward-to` to aggregate logs from multiple remote server probes into a single control panel.

For complete configuration and cluster deployment guides, please refer to the [📚 Usage Guide (docs/usage.md)](docs/usage.md).

## 🚀 Quick Start

### One-Click Deployment

For server or background environments, we provide a one-click deployment script that automatically compiles and registers a Systemd background service:

```bash
chmod +x ./scripts/deploy.sh
sudo ./scripts/deploy.sh
```

> 💡 **Tip**: The script is interactive and allows you to choose between Receiver (Server) and Forwarder (Probe) modes.

### Manual Build & Run

```bash
# Build
go build -o dashboard ./cmd/dashboard

# TUI mode — keep it in a terminal sidebar or Tmux split
./dashboard

# Web mode — open http://localhost:9100 in your browser
./dashboard --web

# Custom port + device ID
./dashboard --web --port 8080 --device-id my-mac
```

### Simulate Radar Trigger

While the dashboard is running, write a mock log line from another terminal:

```bash
echo '{"type":"assistant", "model": "claude-sonnet-4-6", "usage": {"input_tokens": 1000, "output_tokens": 500, "cache_read_input_tokens": 200}}' >> session.jsonl
```

> The HUD / Web dashboard will instantly pick up the new event and persist it to the database.

## ⚙️ Pricing Configuration

Model pricing is embedded into the binary via `cmd/dashboard/pricing_table.json` — no external files needed at runtime. To update prices, edit that file and rebuild:

```json
{
  "models": {
    "gemini-2.5-pro": {
      "input_price_per_m": 1.25,
      "cached_price_per_m": 0.31,
      "output_price_per_m": 5.00
    },
    "claude-sonnet-4-6": {
      "input_price_per_m": 3.00,
      "cached_price_per_m": 0.30,
      "output_price_per_m": 15.00
    }
  }
}
```

## 🏗 Architecture

```
cmd/dashboard/        CLI entry + wiring + embedded pricing_table.json
internal/
├── model/            Shared data types (TokenUsage)
├── watcher/          fsnotify live watcher + JSONL parser
├── scanner/          Historical log bulk/incremental scanner
├── calculator/       Token → USD pricing engine
├── db/               SQLite persistence (WAL mode)
└── web/              HTTP API + embedded React SPA (go:embed)
```

All modules are built with TDD and independently testable:

| Module | Purpose | Tests |
|---|---|:---:|
| `model` | Shared `TokenUsage` struct | — |
| `watcher` | fsnotify watcher + Claude/Gemini log parsing | ✅ |
| `scanner` | Historical log scanning with incremental offsets + truncation detection | ✅ |
| `calculator` | Per-model cost calculation, supports file and byte-stream init | ✅ |
| `db` | SQLite persistence with time-window and device-filtered queries | ✅ |
| `web` | REST API (`/api/stats`) + static file serving | ✅ |

## 📡 API

```
GET /api/stats                # All stats
GET /api/stats?device=my-mac  # Filter by device
```

Returns `{ periods, sources, devices }` — see [dashboard-api.md](dashboard-api.md) for details.

## 🗺 Roadmap

- [x] **Phase 1: HUD Layer** — Persistent Bubble Tea terminal panel with live flickering updates
- [x] **Phase 2: Structured Persistence** — Real-time log interception + SQLite + incremental scanning
- [x] **Phase 2.5: Web Dashboard** — React SPA + HTTP API + embedded distribution
- [ ] **Phase 3: Full Terminal Dashboard** — `Tab` to switch, render ASCII charts and project cost leaderboards

## 📜 License

MIT
