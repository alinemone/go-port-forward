# pf - Port Forward Manager v2.0

Modern CLI tool for managing multiple port-forward connections with beautiful TUI.

## ✨ Features

- 🎨 **Modern TUI** - Beautiful terminal interface powered by Bubbletea
- ⚡ **Fast Detection** - Detects connection failures in 2-4 seconds
- 🔄 **Auto-Reconnection** - Automatically reconnects on failure
- 📊 **Real-time Monitoring** - Live status updates and health checks
- 🎯 **Error Tracking** - Smart error display with auto-clear
- 📝 **Logging** - Rotating logs in `logs/pf.log` with full error details
- 🎨 **Themes** - Multiple color themes (embedded in binary)
- 🧹 **Clean Shutdown** - Properly kills all child processes on exit
- 📦 **Single Binary** - No external dependencies, themes embedded

## Install

### From Source

#### Windows
```bash
go build -o pf.exe
# Move to C:\pf and add to PATH
```

#### Linux/macOS
```bash
go build -o pf
sudo mv pf /usr/local/bin/
sudo chmod +x /usr/local/bin/pf
```

### From Releases

Download pre-built binaries from [Releases](https://github.com/alinemone/go-port-forward/releases).

## Usage

```bash
pf <command> [arguments]
```

### Commands

| Command | Alias | Description |
|---------|-------|-------------|
| `add`   | `a`   | Add new service |
| `list`  | `l`   | List all services |
| `run`   | `r`   | Run services with TUI |
| `delete`| `d`   | Delete service |
| `themes`|       | List available themes |
| `help`  | `h`   | Show help |

## Examples

### Add a service
```bash
pf add db "kubectl -n prod port-forward service/postgres 5432:5432"
pf add redis "kubectl port-forward service/redis 6379:6379"
```

### List services
```bash
pf list
```

### Run services
```bash
pf run db             # Run single service
pf run db,redis       # Run multiple services
```

### Delete service
```bash
pf delete db
```


## TUI Controls

- **q** or **Ctrl+C** - Quit and stop all services
- **r** - Manual refresh

## Configuration

Create a `config.json` file in the same directory as the executable:

```json
{
  "health_check_interval": 2,
  "health_check_timeout": 1,
  "health_check_fail_count": 2,
  "error_auto_clear_delay": 3,
  "ui_refresh_rate": 100,
  "log_max_size": 10,
  "log_max_backups": 3
}
```

### Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `health_check_interval` | How often to check health (seconds) | 2 |
| `health_check_timeout` | Timeout for each check (seconds) | 1 |
| `health_check_fail_count` | Failures before marking as ERROR | 2 |
| `error_auto_clear_delay` | Delay before clearing errors (seconds) | 3 |
| `ui_refresh_rate` | UI refresh rate (milliseconds) | 100 |
| `log_max_size` | Max log file size (MB) | 10 |
| `log_max_backups` | Number of log backups to keep | 3 |

## Architecture

```
internal/
├── app/           → Application coordinator
├── config/        → Config & theme management
├── logger/        → Rotating log system
├── service/       → Service manager, runner, health checker
├── storage/       → Persistence layer
└── ui/            → Bubbletea TUI components

pkg/
└── netutil/       → Network utilities

themes/
├── default.json   → Default cyan theme
├── dark.json      → Dark purple theme
└── light.json     → Light theme
```

## How It Works

1. **Service Storage**: Services are saved in `services.json` with backward compatibility
2. **Health Checking**: TCP port checks every 2 seconds (configurable)
3. **Auto-Reconnection**: Automatic retry on failure with 2-second delay
4. **Process Management**: Proper cleanup of child processes on exit
5. **Logging**: All events logged to `logs/pf.log` with rotation

## Security & Antivirus Notice

This tool executes system commands (kubectl, ssh) and manages network connections, which may trigger antivirus false positives.

**What we do:**
- Execute port-forward commands you provide
- Monitor and reconnect dropped connections
- Save configurations locally

**To resolve:**
- Build from source to verify the code
- Add exception in antivirus software
- Code is open source - audit anytime

## Troubleshooting

### Processes not killed on exit
Fixed in v2.0! Uses process groups for proper cleanup.

### Connection detection too slow
Adjust `health_check_interval` in config.json (minimum: 1 second).

### Logs growing too large
Configure `log_max_size` and `log_max_backups` in config.json.

## Development

### Requirements
- Go 1.21+
- Dependencies: bubbletea, lipgloss, lumberjack

### Build
```bash
go build -o pf
```

### Test
```bash
go test ./...
```

---

**Simple. Fast. Reliable.**
