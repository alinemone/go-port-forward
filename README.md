# pf - Port Forward Manager v2.*

Modern CLI tool for managing multiple port-forward connections with real-time monitoring and certificate support.

## ✨ Features

- 🎨 **Simple TUI** - Clean terminal interface with real-time status
- ⚡ **Fast & Reliable** - Detects connection failures quickly
- 🔄 **Auto-Reconnection** - Automatically reconnects on failure
- 🧹 **Port Cleanup** - Automatically kills conflicting processes
- 🔐 **Certificate Support** - Built-in P12 certificate handling for kubectl
- 📊 **Real-time Monitoring** - Live status updates
- 🛡️ **Graceful Shutdown** - Proper cleanup on exit or Ctrl+C
- 📦 **Single Binary** - No external dependencies
- 🌍 **Cross-Platform** - Works on Windows, Linux, and macOS

## 📥 Installation

### From Releases (Recommended)

#### 🪟 Windows

1. Download `pf-windows-amd64.exe` from [Releases](https://github.com/alinemone/go-port-forward/releases)
2. Open **PowerShell** in the folder where you downloaded it and run this one command. It
   creates `~/.pf` if missing, renames the binary to `pf.exe`, moves it there, and adds
   `~/.pf` to your user PATH:
   ```powershell
   $dest = "$HOME\.pf"
   New-Item -ItemType Directory -Force -Path $dest | Out-Null
   Move-Item -Force .\pf-windows-amd64.exe "$dest\pf.exe"
   $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
   if ($userPath -notlike "*$dest*") {
       $newPath = if ([string]::IsNullOrEmpty($userPath)) { $dest } else { "$userPath;$dest" }
       [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
   }
   Write-Host "Installed to $dest\pf.exe - open a NEW terminal, then run: pf"
   ```
3. **Close and reopen** your terminal (PATH changes only apply to new sessions), then run
   from anywhere:
   ```cmd
   pf
   ```

<details>
<summary>Prefer to do it manually?</summary>

1. Rename `pf-windows-amd64.exe` to `pf.exe` (right-click → **Rename**, or `ren pf-windows-amd64.exe pf.exe`)
2. Move `pf.exe` into a folder that is already in your PATH
3. Run `pf` from anywhere
</details>

#### 🐧 Linux (Intel/AMD)

1. Download `pf-linux-amd64` from [Releases](https://github.com/alinemone/go-port-forward/releases)
2. Rename and install in one command:
   ```bash
   mv pf-linux-amd64 pf && chmod +x pf && sudo mv pf /usr/local/bin/
   ```
3. Run from anywhere:
   ```bash
   pf
   ```

#### 🐧 Linux (ARM/ARM64)

1. Download `pf-linux-arm64` from [Releases](https://github.com/alinemone/go-port-forward/releases)
2. Rename and install in one command:
   ```bash
   mv pf-linux-arm64 pf && chmod +x pf && sudo mv pf /usr/local/bin/
   ```
3. Run from anywhere:
   ```bash
   pf
   ```

#### 🍎 macOS (Intel)

1. Download `pf-darwin-amd64` from [Releases](https://github.com/alinemone/go-port-forward/releases)
2. Rename and install in one command:
   ```bash
   mv pf-darwin-amd64 pf && chmod +x pf && sudo mv pf /usr/local/bin/
   ```
3. Run from anywhere:
   ```bash
   pf
   ```

#### 🍎 macOS (Apple Silicon M1/M2/M3)

1. Download `pf-darwin-arm64` from [Releases](https://github.com/alinemone/go-port-forward/releases)
2. Rename and install in one command:
   ```bash
   mv pf-darwin-arm64 pf && chmod +x pf && sudo mv pf /usr/local/bin/
   ```
3. Run from anywhere:
   ```bash
   pf
   ```

### From Source

#### Windows (PowerShell)
```powershell
go build -trimpath -buildvcs=false -ldflags="-s -w -X github.com/alinemone/go-port-forward/internal/version.Version=dev -X github.com/alinemone/go-port-forward/internal/version.Commit=local -X github.com/alinemone/go-port-forward/internal/version.BuildDate=local" -o pf.exe ./cmd/pf
```
#### Optional: Move build file to a directory in Windows PATH

#### Windows EXE Icon (release build)

To embed a custom icon into `pf-windows-amd64.exe`:

1. Put your icon at `assets/app.ico`
2. Install rsrc once:
   ```bash
   go install github.com/akavel/rsrc@latest
   ```
3. Run release build script:
   ```powershell
   ./scripts/build-release.ps1 -Version vX.Y.Z
   ```

If `assets/app.ico` does not exist, build works normally without a custom icon.

#### Linux/macOS
```bash
go build -trimpath -buildvcs=false -ldflags="-s -w -X github.com/alinemone/go-port-forward/internal/version.Version=dev -X github.com/alinemone/go-port-forward/internal/version.Commit=local -X github.com/alinemone/go-port-forward/internal/version.BuildDate=local" -o pf ./cmd/pf
sudo mv pf /usr/local/bin/
sudo chmod +x /usr/local/bin/pf
```

#### Build All Targets Locally
```bash
LDFLAGS="-s -w -X github.com/alinemone/go-port-forward/internal/version.Version=dev -X github.com/alinemone/go-port-forward/internal/version.Commit=local -X github.com/alinemone/go-port-forward/internal/version.BuildDate=local"

# Windows amd64
GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="$LDFLAGS" -o pf-windows-amd64.exe ./cmd/pf

# Linux amd64
GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="$LDFLAGS" -o pf-linux-amd64 ./cmd/pf

# Linux arm64
GOOS=linux GOARCH=arm64 go build -trimpath -buildvcs=false -ldflags="$LDFLAGS" -o pf-linux-arm64 ./cmd/pf

# macOS amd64 (Intel)
GOOS=darwin GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="$LDFLAGS" -o pf-darwin-amd64 ./cmd/pf

# macOS arm64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -trimpath -buildvcs=false -ldflags="$LDFLAGS" -o pf-darwin-arm64 ./cmd/pf
```

## 🚀 Quick Start

```bash
# Add a service
pf add db "kubectl port-forward service/postgres 5432:5432"

# Run the service
pf run db

# Run any kubectl command with auto certificate injection
pf k get pods -n production

# List all services
pf list
```

## 📖 Commands

| Command | Alias | Description |
|---------|-------|-------------|
| `add`   | `a`   | Add new service |
| `list`  | `l`   | List all services |
| `kubectl` | `k` | Run any kubectl command with configured certificate |
| `run`   | `r`   | Run services with TUI |
| `delete`| `d`   | Delete service |
| `rename`| `ren`, `mv` | Rename a service or group |
| `edit`  |       | Bulk-edit all services/groups in `$EDITOR` |
| `cleanup`| `c`  | Free configured ports (`--all` kills all kubectl/ssh) |
| `group` | `g`   | Manage groups (add/add-service/remove-service/list/delete/rename) |
| `cert`  |       | Manage certificates (add/list/remove) |
| `version`  | `v`  | Show build version details |
| `help`  | `h`   | Show help |

## 🔐 Certificate Management

Add P12 certificates for secure kubectl connections:

```bash
# Add certificate (used for all kubectl services)
pf cert add "/path/to/certificate.p12"

# View configured certificate
pf cert list

# Remove certificate
pf cert remove
```

**How it works:**
- Extracts certificate and private key from P12 file
- Stores them securely in `~/.pf/certs/`
- Automatically injects `--client-certificate` and `--client-key` flags into kubectl service commands and `pf k ...` / `pf kubectl ...`
- Password is only required during setup (not stored)

## 💡 Usage Examples

### Basic Service Management

```bash
# Add services
pf add db "kubectl -n prod port-forward service/postgres 5432:5432"
pf add redis "kubectl port-forward service/redis 6379:6379"
pf add api "kubectl port-forward deployment/api 8080:8080"

# List all services
pf list

# Run single service
pf run db

# Run multiple services
pf run db,redis,api

# Delete a service
pf delete redis
```

### With Certificate

```bash
# 1. Add your P12 certificate
pf cert add company-vpn.p12
# Enter password when prompted

# 2. Add kubectl service (certificate will be auto-used)
pf add production "kubectl -n prod port-forward service/postgres 5432:5432"

# 3. Run the service
pf run production
```

### Rename a Service or Group

```bash
# Rename a service (group memberships are updated automatically)
pf rename db database

# Rename a group explicitly
pf group rename backend back
```

### Add / Remove Services in a Group

```bash
# Add services to an existing group (deduplicated)
pf group add-service database wallet-pg,redis

# Remove services from a group
pf group remove-service database redis
```

### Bulk Edit Configuration

```bash
# Open all services and groups in your $EDITOR (vim/nano/notepad...)
pf edit
```
Great for the initial setup when adding many services at once. The file is
validated on save; invalid JSON is rejected and you are offered to reopen and fix it.

### Cleanup Stuck Ports

```bash
# Free only the local ports used by your configured services
pf cleanup

# Kill ALL kubectl/ssh processes on the machine (asks for confirmation; -y to skip)
pf cleanup --all
```

## 🎮 TUI Controls

When running services:

- **↑↓** / **j k** - Move selection between services
- **PgUp** / **PgDn** / **mouse wheel** - Scroll the log panel
- **l** - Toggle the log panel between all services and only the selected service
- **r** - Restart the selected service
- **Ctrl+R** - Restart all services
- **s** - Stop the selected service
- **a** - Add another stored service to the running set
- **e** - Bulk-edit configuration in `$EDITOR`
- **q** / **Esc** / **Ctrl+C** - Quit and stop all services

## 📂 File Locations

```
~/.pf/
├── certificate.json      → Certificate configuration
├── services.json         → Stored services and groups
└── certs/
    ├── client-cert.pem   → Extracted certificate
    └── client-key.pem    → Private key
```

> On first run, an existing `services.json` next to the executable (legacy
> location) is automatically migrated to `~/.pf/services.json`.
>
> On Windows, the recommended installer also places the binary itself at
> `~/.pf/pf.exe` (i.e. `C:\Users\<you>\.pf\pf.exe`).

## 🏗️ Architecture

```
.
├── cmd/pf/
│   └── main.go              → CLI entry point and commands
├── internal/
│   ├── model/service.go     → Service types and status constants
│   ├── stringutil/normalize.go → Input normalization
│   ├── version/version.go   → Build version info
│   ├── storage/storage.go   → Service persistence, groups, rename, migration
│   ├── configedit/          → $EDITOR bulk-edit + config validation
│   ├── manager/
│   │   ├── manager.go       → Service lifecycle, health probe, auto-reconnect
│   │   ├── output.go        → Output classification
│   │   ├── port.go          → Port-listener discovery for targeted cleanup
│   │   ├── proc_unix.go     → Unix process groups / port cleanup
│   │   └── proc_windows.go  → Windows process groups / port cleanup
│   ├── ui/ui.go             → Terminal UI (Bubbletea)
│   └── cert/
│       ├── p12.go           → P12 certificate extraction
│       └── manager.go       → Certificate management
```

## 🔧 How It Works

1. **Port Management**: Automatically detects and kills processes using target ports
2. **Service Storage**: Services saved in `~/.pf/services.json`
3. **Auto-Reconnection**: Reconnects when the process exits or kubectl reports a fatal error, using capped exponential backoff — never permanently gives up, and resets backoff after a connection stays healthy. No extra connections are made to your backend.
4. **Certificate Injection**: For kubectl commands, automatically adds certificate flags
5. **Process Cleanup**: Proper cleanup of all processes on exit

## 🛡️ Security

### Certificate Handling
- P12 password is only used during extraction (never stored)
- Certificate and key files stored with `0600` permissions (owner-only)
- Files stored in user's home directory (`~/.pf/`)

### Antivirus Notice
This tool executes system commands (kubectl, ssh) and manages network connections, which may trigger antivirus false positives.

**Recommendations:**
- Build from source (or trusted CI artifacts) and verify checksums
- Use reproducible build flags: `-trimpath -buildvcs=false -ldflags="-s -w"`
- Avoid packers/obfuscators like UPX
- Submit false-positive reports to your antivirus vendor
- Add exception in antivirus software if needed
- Code is open source - audit anytime

## 🐛 Troubleshooting

### Port already in use
Run `pf cleanup` to kill all kubectl/ssh processes.

### Certificate not working
- Verify the P12 file path is correct
- Ensure you entered the correct password
- Check certificate with: `pf cert list`

### Service won't start
- Check the command manually: `kubectl port-forward ...`
- Verify kubectl context and permissions
- Check if certificate is required

## 💻 Development

### Requirements
- Go 1.21+
- Dependencies:
  - `github.com/charmbracelet/bubbletea` - TUI framework
  - `github.com/charmbracelet/lipgloss` - Styling
  - `software.sslmate.com/src/go-pkcs12` - P12 handling

### Build
```bash
go build -trimpath -buildvcs=false -ldflags="-s -w -X github.com/alinemone/go-port-forward/internal/version.Version=dev -X github.com/alinemone/go-port-forward/internal/version.Commit=local -X github.com/alinemone/go-port-forward/internal/version.BuildDate=local" -o pf ./cmd/pf
```

### Cross-Platform Build
```bash
LDFLAGS="-s -w -X github.com/alinemone/go-port-forward/internal/version.Version=dev -X github.com/alinemone/go-port-forward/internal/version.Commit=local -X github.com/alinemone/go-port-forward/internal/version.BuildDate=local"

# Windows
GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="$LDFLAGS" -o pf1.exe ./cmd/pf

# Linux amd64
GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="$LDFLAGS" -o pf-linux-amd64 ./cmd/pf

# Linux arm64
GOOS=linux GOARCH=arm64 go build -trimpath -buildvcs=false -ldflags="$LDFLAGS" -o pf-linux-arm64 ./cmd/pf

# macOS amd64
GOOS=darwin GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="$LDFLAGS" -o pf-darwin-amd64 ./cmd/pf

# macOS arm64
GOOS=darwin GOARCH=arm64 go build -trimpath -buildvcs=false -ldflags="$LDFLAGS" -o pf-darwin-arm64 ./cmd/pf
```

### Local Release Script (No Paid Signing)
```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-release.ps1 -Version v2.1.0
```
This generates all release binaries in `dist/` and creates `dist/SHA256SUMS.txt`.

## 🤝 Contributing

Contributions are welcome! Feel free to:
- Report bugs
- Suggest features
- Submit pull requests

## 📄 License

Open source - feel free to use and modify.

---

**Simple. Secure. Reliable.**
