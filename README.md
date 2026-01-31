# GS Panel

A lightweight, self-hosted game server management panel built with Go + HTMX.

## Why GS Panel?

Other game server panels exist, but they often feel **over-engineered**:

- **Too many dependencies** - Node.js, PHP, databases, message queues
- **Complex setup** - Multi-step installation, external services required
- **Heavy resource usage** - 500MB+ RAM just for the panel
- **Docker complexity** - Nested containers, confusing networking
- **Feature bloat** - You don't need 80% of the features

**GS Panel** takes a different approach:

- **Single binary** - Just one executable, no dependencies
- **SQLite database** - No external database to configure
- **~50MB RAM** - Runs on a Raspberry Pi
- **Simple Docker** - One container, volume mounts
- **Just works** - Create server → Start playing

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

### Binary

```bash
# Download binary
wget https://github.com/sneakykiwi/gs-panel/releases/latest/download/gs-panel-linux-amd64
chmod +x gs-panel-linux-amd64

# Run
./gs-panel-linux-amd64
```

Then open `http://localhost:8080` and complete the setup.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        USER BROWSER                          │
└──────────────────────┬──────────────────────────────────────┘
                       │ HTTP/WebSocket
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                    GS PANEL (Go Binary)                      │
│  ┌─────────────────────────────────────────────────────────┐│
│  │  HTTP Router (Fiber)                                    ││
│  │  ├── Rate Limiting                                      ││
│  │  ├── CSRF Protection                                    ││
│  │  └── Session Management                                 ││
│  └─────────────────────────────────────────────────────────┘│
│                         │                                    │
│  ┌──────────────────────┼──────────────────────────────────┐│
│  │                      ▼                                   ││
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ││
│  │  │   Handlers   │  │   Services   │  │   WebSocket  │  ││
│  │  │  - Auth      │  │  - Server    │  │  - Console   │  ││
│  │  │  - Server    │  │  - Backup    │  │  - Stats     │  ││
│  │  │  - Files     │  │  - Files     │  │              │  ││
│  │  │  - Admin     │  │              │  │              │  ││
│  │  └──────────────┘  └──────────────┘  └──────────────┘  ││
│  └─────────────────────────────────────────────────────────┘│
│                         │                                    │
│  ┌──────────────────────┼──────────────────────────────────┐│
│  │                      ▼                                   ││
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ││
│  │  │   SQLite     │  │   Docker     │  │   File       │  ││
│  │  │   Database   │◄─┤   Client     │  │   System     │  ││
│  │  │              │  │              │  │              │  ││
│  │  │ - Users      │  │ - Containers │  │ - Servers    │  ││
│  │  │ - Servers    │  │ - Images     │  │ - Backups    │  ││
│  │  │ - Backups    │  │ - Logs       │  │              │  ││
│  │  └──────────────┘  └──────────────┘  └──────────────┘  ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
                       │
                       ▼ Docker API
┌─────────────────────────────────────────────────────────────┐
│                     DOCKER DAEMON                            │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   Minecraft  │  │   Terraria   │  │   Valheim    │  ... │
│  │   Container  │  │   Container  │  │   Container  │      │
│  │              │  │              │  │              │      │
│  │ - Port: 25565│  │ - Port: 7777 │  │ - Port: 2456 │      │
│  │ - RAM: 2GB   │  │ - RAM: 1GB   │  │ - RAM: 4GB   │      │
│  │ - Vol: data  │  │ - Vol: data  │  │ - Vol: data  │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

## Features

### Core
- ✅ **Server Management** - Create, start, stop, restart game servers
- ✅ **Real-time Console** - View logs and send commands via WebSocket
- ✅ **File Manager** - Browse, edit, upload, download server files
- ✅ **Backup System** - Manual and scheduled backups with restore
- ✅ **User Management** - Multi-user with role-based access

### Security
- ✅ **Rate Limiting** - Protect against abuse (login, backups, commands)
- ✅ **CSRF Protection** - Prevent cross-site request forgery
- ✅ **Password Hashing** - bcrypt with cost 12
- ✅ **Session Management** - Secure HTTP-only cookies
- ✅ **Path Validation** - Prevent directory traversal attacks

### Games Supported
- Minecraft Java & Bedrock
- Terraria
- Valheim
- Palworld
- (Easily extensible via templates)

## Project Structure

```
gs-panel/
├── cmd/server/           # Main entry point
│   └── main.go
├── internal/
│   ├── config/          # Configuration
│   ├── database/        # Database connection
│   ├── forms/           # Form structs
│   ├── handlers/        # HTTP handlers
│   ├── middleware/      # Auth, rate limiting
│   ├── models/          # Database models
│   ├── services/        # Business logic
│   ├── validators/      # Validation logic
│   └── logger/          # Structured logging
├── web/
│   ├── templates/       # HTML templates
│   └── static/          # CSS, JS assets
├── Dockerfile           # Container image
└── README.md           # This file
```

## Development

### Prerequisites
- Go 1.21+
- Docker (for testing game servers)

### Build

```bash
# Build binary
go build -o bin/server cmd/server/main.go

# Build with version
CGO_ENABLED=1 go build -ldflags "-X main.version=1.0.0" -o bin/server cmd/server/main.go
```

### Run

```bash
# Development
./bin/server

# With custom config
./bin/server -config config.yaml
```

## Configuration

Environment variables or `config.yaml`:

```yaml
server:
  host: 0.0.0.0
  port: 8080

database:
  path: ./data/gs-panel.db

storage:
  servers: ./data/servers
  backups: ./data/backups
  logs: ./data/logs
```

## Comparison

| Feature | GS Panel | Pterodactyl | AMP | LinuxGSM |
|---------|----------|-------------|-----|----------|
| **Setup Time** | 5 min | 30+ min | 20 min | 15 min |
| **Dependencies** | None | PHP, MySQL, Redis | .NET | None |
| **RAM Usage** | ~50MB | ~500MB | ~300MB | ~10MB |
| **Database** | SQLite (built-in) | MySQL/PostgreSQL | SQLite | None |
| **Web UI** | ✅ | ✅ | ✅ | ❌ |
| **Docker** | Simple | Complex | Simple | N/A |
| **Backups** | ✅ | ✅ | ✅ | Manual |
| **Multi-user** | ✅ | ✅ | ✅ | ❌ |

## Roadmap

### Completed ✅
- [x] Server lifecycle management
- [x] Real-time console with WebSocket
- [x] File manager (CRUD operations)
- [x] Backup/restore system
- [x] User authentication & RBAC
- [x] Rate limiting & CSRF protection
- [x] Game templates (Minecraft, Terraria, Valheim, Palworld)

### Planned
- [ ] 2FA/TOTP support
- [ ] Server monitoring dashboards
- [ ] Plugin system
- [ ] API for external integrations
- [ ] Mobile-friendly UI improvements
- [ ] Additional game templates

## License

MIT License
