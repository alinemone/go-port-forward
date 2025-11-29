# pf - Port Forward Manager

Minimal CLI tool for managing multiple port-forward connections.

## Install

### Option 1: Download Pre-built Binary (Recommended)

No need to install Go! Download the binary for your platform from [Releases](https://github.com/alinemone/go-port-forward/releases):

#### Windows
```powershell
# Download pf-windows-amd64.exe from releases
# Rename to pf.exe
# Move to C:\pf
# Add C:\pf to PATH
```

#### Linux (AMD64)
```bash
# Download the latest release
curl -L -o pf https://github.com/alinemone/go-port-forward/releases/latest/download/pf-linux-amd64

# Install
sudo mv pf /usr/local/bin/
sudo chmod +x /usr/local/bin/pf
```

#### Linux (ARM64)
```bash
curl -L -o pf https://github.com/alinemone/go-port-forward/releases/latest/download/pf-linux-arm64
sudo mv pf /usr/local/bin/
sudo chmod +x /usr/local/bin/pf
```

#### macOS (Intel)
```bash
curl -L -o pf https://github.com/alinemone/go-port-forward/releases/latest/download/pf-darwin-amd64
sudo mv pf /usr/local/bin/
sudo chmod +x /usr/local/bin/pf
```

#### macOS (Apple Silicon)
```bash
curl -L -o pf https://github.com/alinemone/go-port-forward/releases/latest/download/pf-darwin-arm64
sudo mv pf /usr/local/bin/
sudo chmod +x /usr/local/bin/pf
```

### Option 2: Build from Source

Requires Go 1.21+

```bash
# Clone the repository
git clone https://github.com/alinemone/go-port-forward.git
cd go-port-forward

# Build for your platform
go build -o pf

# Or cross-compile for other platforms:
# GOOS=windows GOARCH=amd64 go build -o pf.exe
# GOOS=linux GOARCH=amd64 go build -o pf
# GOOS=darwin GOARCH=arm64 go build -o pf
```

Then follow the installation steps above for your platform.

---

Now use `pf` from anywhere! 🚀

## Usage

```bash
pf <command> [arguments]
```

### Commands

| Command | Alias | Description |
|---------|-------|-------------|
| `add`   | `a`   | Add new service |
| `list`  | `l`   | List all services |
| `run`   | `r`   | Run services |
| `delete`| `d`   | Delete service |
| `help`  | `h`   | Show help |

## Examples

### Add a service
```bash
pf a db_name "kubectl -n prod port-forward service/postgres 5432:5432"
```

### List services
```bash
pf l
```

### Run services
```bash
pf r db          # Run single service
pf r db,web      # Run multiple services
```

### Delete service
```bash
pf d db
```

## How it works

Services are saved in `services.json` and automatically reconnect if the connection drops.

---

**Simple. Fast. Reliable.**
