# Game Server Templates

This directory contains the default game server templates that ship with GS Panel.

## Template Structure

Templates are YAML files that define how a game server container should be created and managed.

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique identifier (lowercase, no spaces, use hyphens) |
| `name` | string | Display name shown in the UI |
| `docker_image` | string | Docker image to use (e.g., `itzg/minecraft-server:latest`) |

### Optional Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `version` | string | `1.0.0` | Template version for tracking updates |
| `default_port` | int | - | Default port for the game server |
| `default_memory` | int | - | Default memory limit in MB |
| `protocol` | string | `both` | Port protocol: `tcp`, `udp`, or `both` |
| `environment` | map | `{}` | Environment variables for the container |
| `stop_command` | string | - | Command to send via console for graceful stop |
| `save_command` | string | - | Command to send via console to save before stop |
| `stop_timeout` | int | `30` | Seconds to wait for graceful shutdown |

### Advanced Fields

| Field | Type | Description |
|-------|------|-------------|
| `additional_ports` | list | Extra ports to expose (see below) |
| `volumes` | list | Additional volume mounts (see below) |
| `entrypoint` | list | Override container entrypoint |
| `cmd` | list | Override container command |
| `health_check` | object | Container health check config |
| `user` | string | Container user (e.g., `1000:1000`) |
| `cap_add` | list | Linux capabilities to add |
| `network_mode` | string | Docker network mode |
| `labels` | map | Container labels |
| `stop_signal` | string | Signal to send for stop (e.g., `SIGTERM`) |
| `var_descriptions` | map | Descriptions for env vars (shown in UI) |

### Additional Ports

```yaml
additional_ports:
  - port: 27015
    protocol: udp  # tcp, udp, or both
    purpose: query  # informational, shown in UI
```

### Volumes

```yaml
volumes:
  - host: backups      # relative to server data dir, or absolute path
    container: /backups
    mode: rw           # rw (read-write) or ro (read-only)
```

### Health Check

```yaml
health_check:
  test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
  interval: 30s
  timeout: 10s
  retries: 3
  start_period: 60s
```

### Variable Substitution

Use `{{MEMORY}}` in environment values to substitute the server's memory limit:

```yaml
environment:
  MAX_MEMORY: "{{MEMORY}}M"
```

## Example Template

```yaml
id: my-game-server
name: My Game Server
version: "1.0.0"
docker_image: gameserver/image:latest
default_port: 27015
default_memory: 4096
protocol: udp

environment:
  SERVER_NAME: "My Server"
  MAX_PLAYERS: "32"
  RCON_PASSWORD: ""

additional_ports:
  - port: 27016
    protocol: tcp
    purpose: rcon

stop_command: quit
save_command: save
stop_timeout: 60

var_descriptions:
  SERVER_NAME: "Name shown in the server browser"
  MAX_PLAYERS: "Maximum number of players"
  RCON_PASSWORD: "Remote console password"
```

## Custom Templates

User-created templates are stored in `data/templates/` and can be:

- Created via the admin UI (`/admin/templates/new`)
- Cloned from built-in templates
- Added manually as YAML files

Custom templates are not overwritten on updates.

## Notes

- Template IDs must be unique across both default and custom templates
- Built-in templates (in this directory) cannot be edited, but can be cloned
- Changes to templates don't affect existing servers until they are upgraded
