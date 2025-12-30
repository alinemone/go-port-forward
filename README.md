# pf - Port Forward Manager v2.1

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
2. Rename to `pf.exe`:
   - **Right-click** the file → **Rename** → Change name to `pf.exe`
   - Or in **Command Prompt/PowerShell**:
     ```cmd
     ren pf-windows-amd64.exe pf.exe
     ```
3. Move `pf.exe` to a folder in your PATH
4. Run from anywhere:
   ```cmd
   pf.exe
   ```

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

#### Windows
```bash
go build -o pf.exe
# Optional: Move to a directory in PATH
```

#### Linux/macOS
```bash
go build -o pf
sudo mv pf /usr/local/bin/
sudo chmod +x /usr/local/bin/pf
```

## 🚀 Quick Start

```bash
# Add a service
pf add db "kubectl port-forward service/postgres 5432:5432"

# Run the service
pf run db

# List all services
pf list
```

## 📖 Commands

| Command | Alias | Description |
|---------|-------|-------------|
| `add`   | `a`   | Add new service |
| `list`  | `l`   | List all services |
| `run`   | `r`   | Run services with TUI |
| `delete`| `d`   | Delete service |
| `cleanup`| `c`  | Kill all kubectl/ssh processes |
| `cert`  |       | Manage certificates (add/list/remove) |
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
- Automatically injects `--client-certificate` and `--client-key` flags into kubectl commands
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

### Cleanup Stuck Ports

```bash
# Kill all kubectl/ssh processes
pf cleanup
```

## 🎮 TUI Controls

When running services:

- **q** or **Ctrl+C** - Quit and stop all services
- **r** - Manual refresh

## 📂 File Locations

```
~/.pf/
├── certificate.json      → Certificate configuration
└── certs/
    ├── client-cert.pem   → Extracted certificate
    └── client-key.pem    → Private key

services.json             → Stored services (same directory as executable)
```

## 🏗️ Architecture

```
.
├── main.go          → CLI entry point and commands
├── manager.go       → Service lifecycle management
├── storage.go       → Service persistence
├── ui.go            → Terminal UI (Bubbletea)
└── cert/
    ├── p12.go       → P12 certificate extraction
    └── manager.go   → Certificate management
```

## 🔧 How It Works

1. **Port Management**: Automatically detects and kills processes using target ports
2. **Service Storage**: Services saved in `services.json`
3. **Auto-Reconnection**: Automatically restarts failed connections
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
- Build from source to verify the code
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
go build -o pf
```

### Cross-Platform Build
```bash
# Windows
GOOS=windows GOARCH=amd64 go build -o pf.exe

# Linux
GOOS=linux GOARCH=amd64 go build -o pf

# macOS
GOOS=darwin GOARCH=amd64 go build -o pf
```

## 🤝 Contributing

Contributions are welcome! Feel free to:
- Report bugs
- Suggest features
- Submit pull requests

## 📄 License

Open source - feel free to use and modify.

---

**Simple. Secure. Reliable.**
