# pf - Port Forward Manager

Minimal CLI tool for managing multiple port-forward connections.

## Install

```bash
go build -o pf.exe
```

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
