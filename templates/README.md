# Game Server Templates

Templates define how game server containers are created and managed.

## Structure

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique identifier (lowercase, hyphens) |
| `name` | string | Display name |
| `docker_image` | string | Docker image to use |

### Common Fields

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Template version |
| `default_port` | int | Default port |
| `default_memory` | int | Default memory (MB) |
| `protocol` | string | `tcp`, `udp`, or `both` |
| `environment` | map | Environment variables |
| `stop_command` | string | Graceful stop command |
| `save_command` | string | Save command before stop |
| `stop_timeout` | int | Shutdown timeout (seconds) |

### Advanced Fields

| Field | Type | Description |
|-------|------|-------------|
| `additional_ports` | list | Extra ports to expose |
| `volumes` | list | Additional volume mounts |
| `entrypoint` | list | Override entrypoint |
| `cmd` | list | Override command |
| `health_check` | object | Health check config |
| `user` | string | Container user |
| `cap_add` | list | Linux capabilities |
| `network_mode` | string | Docker network mode |

## Variables

Use these placeholders in environment values and volume mounts:

| Variable         | Description                          |
|------------------|--------------------------------------|
| `{{MEMORY}}`     | Memory limit (MB)                    |
| `{{PORT}}`       | Server port                          |
| `{{SERVER_ID}}`  | Server UUID                          |
| `{{SERVER_NAME}}`| Server name                          |
| `{{SERVER_DIR}}` | Server data directory path           |
| `{{BACKUP_DIR}}` | Server backup directory path (opt.)  |
| `{{LOGS_DIR}}`   | Server logs directory path (opt.)    |

## Example

```yaml
id: my-game
name: My Game Server
version: "1.0.0"
docker_image: gameserver/image:latest
default_port: 27015
default_memory: 4096
protocol: udp

environment:
  SERVER_NAME: "{{SERVER_NAME}}"
  MAX_MEMORY: "{{MEMORY}}M"

# Volumes can use either object or short syntax:
volumes:
  # Short syntax
  - "{{SERVER_DIR}}:/config"
  - "{{BACKUP_DIR}}:/backups:ro"
  # Object syntax
  - host: "{{LOGS_DIR}}"
    container: "/logs"
    mode: rw

stop_command: quit
save_command: save
stop_timeout: 60
```

## Custom Templates

Create custom templates via:
- Admin UI: `/admin/templates/new`
- Clone existing templates
- Add YAML files to `data/templates/`

Custom templates are preserved on updates.
