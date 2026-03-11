# Server Scanner

Scans all Tailscale hosts and generates system reports.

## Usage

```bash
./scan-servers.sh [output-dir]
```

Default output directory: `../../svr-rpts` (relative to script location)

## What it collects

For each reachable host:

- **System info**: hostname, OS, kernel, uptime, architecture
- **CPU**: model, cores, load averages
- **Memory**: total, used, available, percentage
- **Disk**: all mounted filesystems with usage
- **User services**: systemd `--user` services (running)
- **Non-standard services**: system services excluding common OS services
- **Docker**: running containers with image, status, ports
- **Docker volumes**: first 20 volume names

## Output

Reports are saved to timestamped directories:

```
svr-rpts/
├── 2026-02-04_143022/
│   ├── 00-SUMMARY.md      # Overview of all hosts
│   ├── trv-srv-001.md     # Individual host report
│   ├── trv-pi-001.md
│   └── ...
└── latest -> 2026-02-04_143022/   # Symlink to most recent
```

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `SSH_USER | <user>` | SSH username for all hosts |

## Requirements

- Tailscale running and connected
- SSH key-based auth to target hosts
- `jq` installed locally
- Target hosts need: `systemctl`, `free`, `df`, optionally `docker`
