# GS Panel

A lightweight, self-hosted game server management panel built with Go + HTMX.

## Quick Start

### Docker (Recommended)

```bash
docker run -d \
  --name gs-panel \
  -p 8080:8080 \
  -v gs-panel-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/sneakykiwi/gs-panel:latest
```

Then open `http://localhost:8080` and complete the setup.

### Binary

> Coming soon

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        USER BROWSER                          │
└──────────────────────────────┬──────────────────────────────┘
                               │ HTTP/WebSocket
                               ▼
┌─────────────────────────────────────────────────────────────┐
│                    GS PANEL (Go Binary)                      │
│                                                              │
│   HTTP Router ─► Handlers ─► Services ─► Docker Client       │
│        │                         │                           │
│        ▼                         ▼                           │
│   SQLite DB              File System (servers, backups)      │
└──────────────────────────────┬──────────────────────────────┘
                               │ Docker API
                               ▼
┌─────────────────────────────────────────────────────────────┐
│                      DOCKER DAEMON                           │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐                  │
│   │ Minecraft│  │ Terraria │  │ Valheim  │  ...             │
│   └──────────┘  └──────────┘  └──────────┘                  │
└─────────────────────────────────────────────────────────────┘
```

## Project Structure

```
gs-panel/
├── cmd/server/          # Entry point
├── internal/
│   ├── config/          # Configuration
│   ├── database/        # SQLite connection
│   ├── handlers/        # HTTP handlers
│   ├── middleware/      # Auth, rate limiting
│   ├── models/          # Database models
│   ├── services/        # Business logic
│   └── logger/          # Structured logging
├── views/               # Templ templates
├── templates/           # Game server templates
├── web/static/          # Static assets
└── Dockerfile
```

## Development

### Prerequisites

- Go 1.21+
- Docker
- [Templ](https://templ.guide/)

### Commands

```bash
# Install dependencies
make deps

# Generate templ files
make templ

# Build and run (dev mode)
make dev

# Build with version info
make build

# Run tests
make test

# Build Docker image
make docker

# Build for all platforms
make release
```

## Configuration

Environment variables:

```bash
# Server
GS_PANEL_HOST=0.0.0.0
GS_PANEL_PORT=8080

# Data directory (default: ./data on Windows, /var/lib/gs-panel on Linux)
GS_PANEL_DATA_DIR=./data

# Or set individual paths
GS_PANEL_DB_PATH=./data/panel.db
GS_PANEL_SERVERS_DIR=./data/servers
GS_PANEL_BACKUPS_DIR=./data/backups
GS_PANEL_LOGS_DIR=./data/logs
```

## License

MIT License
