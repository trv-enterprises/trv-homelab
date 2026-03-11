# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This repository consolidates all homelab infrastructure: EdgeLake node deployments, Open Horizon service management, ts-store instances, MQTT broker configs, and development tools. It replaces the previous `utilities` and `trv-edgelake-infra` repositories.

EdgeLake/AnyLog documentation is located at: `/path/to/documentation`

## Directory Layout

- `hub/` -- Centralized infrastructure running on trv-srv-001
  - `hub/edgelake/` -- Docker-compose deployments for master and query nodes
  - `hub/open-horizon/` -- OH management hub configs and Makefile
- `edge/` -- Generic deployment configs, deployable to any device
  - `edge/edgelake-operator/docker-compose/` -- Docker-compose for operator nodes
  - `edge/edgelake-operator/open-horizon/` -- OH-managed operator deployment
  - `edge/edgelake-operator/helm-charts/` -- Kubernetes Helm chart
  - `edge/edgelake-operator/kube-operator-go/` -- Go-based Kubernetes operator
  - `edge/zigbee2mqtt/` -- Zigbee2MQTT deployment (services LXC, Docker Compose)
  - `edge/tsstore/` -- ts-store deployment configs
  - `edge/mosquitto/` -- MQTT broker deployment
- `devices/` -- Per-device instance configs (thin, reference edge/ configs)
- `simulators/` -- Dashboard data source simulator deployment
  - `simulators/deploy.yml` -- Ansible playbook for LXD container deployment
  - `simulators/env/` -- Environment files (production.env contains secrets)
- `tools/` -- Development and testing utilities
  - `tools/mcp-client/` -- Full-stack MCP chat application (FastAPI + React)
  - `tools/mcp-server/` -- EdgeLake MCP server build/deploy Makefile
  - `tools/data-loader/` -- Sample data loading scripts
- `docs/` -- Architecture docs, deployment guides, MCP exploratory notes

## Architecture

### EdgeLake Docker Compose (hub/ and edge/)

Both `hub/edgelake/` and `edge/edgelake-operator/docker-compose/` use the same **Docker Compose override pattern**:

```
├── docker-compose.base.yml          # Shared base configuration
├── Makefile                         # Primary interface for all operations
├── .env.root.template              # Environment-wide defaults
└── deployments/
    └── <type>/                     # Per-node deployment
        ├── docker-compose.base.yml          # Symlink to ../../docker-compose.base.yml
        ├── docker-compose.override.yml      # Deployment-specific compose config
        ├── .env                             # Deployment overrides
        └── configs/
            ├── base_configs.env             # Core node settings
            └── advance_configs.env          # Advanced settings
```

**Hub deployments**: master, query (run on trv-srv-001)
**Edge deployments**: operator, operator2 (run on trv-srv-012 and other edge nodes)

### Configuration Priority (highest to lowest)
1. Command-line variables
2. Shell environment variables
3. `.env` file (deployment-specific)
4. `configs/advance_configs.env`
5. `configs/base_configs.env`

### Node Types
- **master**: Blockchain coordinator (ports 32048/32049)
- **query**: Query execution node (ports 32348/32349)
- **operator**: Data storage/processing nodes (ports 32148/32149)

## Commands

### EdgeLake Docker Compose (hub or edge)

```bash
# From hub/edgelake/ or edge/edgelake-operator/docker-compose/
make up <type>           # Start deployment
make down <type>         # Stop deployment
make restart <type>      # Restart deployment
make clean <type>        # Stop and remove volumes (deletes data!)
make logs <type>         # View logs (follow mode)
make attach <type>       # Attach to EdgeLake CLI (Ctrl-D to detach)
make connect <type>      # Open bash shell in container
make status <type>       # Show deployment status
make info <type>         # Show deployment configuration
make sync <type>         # Sync deployment to remote host
make pull <type>         # Pull image from registry

# Batch operations
make up-all              # Start all deployments
make down-all            # Stop all deployments
make status-all          # Show status of all deployments
```

### Open Horizon Service Publishing

```bash
# From hub/open-horizon/
make -f Makefile.oh oh-publish-all     # Publish service + policies
make -f Makefile.oh oh-list-services   # List published services
make -f Makefile.oh sync               # Sync to hub server
make -f Makefile.oh oh-check-vars      # Validate configuration
```

### MCP Server Build/Deploy

```bash
# From tools/mcp-server/
make build arch=arm64    # Build locally for arm64
make build arch=amd64    # Build on remote VM for amd64
make deploy              # Deploy to target host
make all arch=arm64      # Build and deploy
make attach query        # Attach to query node CLI
```

### Zigbee2MQTT (Services LXC)

```bash
# From edge/zigbee2mqtt/
make deploy              # Sync compose + config, restart full stack
make deploy-config       # Sync Z2M config only (WARNING: overwrites network_key)
make logs                # View zigbee2mqtt logs (follow mode)
make status              # Show docker compose service status
make restart             # Restart zigbee2mqtt container
make permit-join         # Enable Zigbee pairing mode via API
make stop-join           # Disable pairing mode
make setup-usb           # Show USB passthrough commands for Proxmox
```

### Simulators (LXD container on trv-srv-001)

```bash
# From simulators/
make deploy              # Full deployment (sync, build, start all)
make status              # Show service status
make logs                # View logs (follow mode)

# Individual services
make up-data-writer      # Start data-writer (50 sensors @ 1/sec)
make down-data-writer    # Stop data-writer
make up-tsstore          # Start tsstore
make down-tsstore        # Stop tsstore

# Note: data-writer uses significant CPU (50 writes/sec to tsstore)
# Keep it stopped unless actively testing
```

## Key Configuration Variables

### EdgeLake Core Settings (base_configs.env)
- `NODE_TYPE`: master, operator, or query
- `NODE_NAME`: Container name (e.g., edgelake-master)
- `ANYLOG_SERVER_PORT`: TCP port for node communication
- `ANYLOG_REST_PORT`: REST API port
- `DB_TYPE`: sqlite or psql
- `LEDGER_CONN`: Master node connection (IP:port)

### EdgeLake Advanced Settings (advance_configs.env)
- `OVERLAY_IP`: Network overlay IP (used for remote SSH/sync)
- `MCP_AUTOSTART`: Auto-start Model Context Protocol server

## Network Details

### Active EdgeLake Network
- **Master**: <hub-tailscale-ip>:32048 (edgelake-master) -- hub/edgelake/
- **Query**: <hub-tailscale-ip>:32348 (edgelake-query) -- hub/edgelake/
- **Operator 1**: <edge-srv-012-tailscale-ip>:32148 (edgelake-operator) -- edge docker-compose on trv-srv-012
- **Operator 2**: <edge-srv-014-tailscale-ip>:32248 (edgelake-operator-oh) -- Open Horizon on trv-srv-014

### Open Horizon Hub
- **Hub Location**: trv-srv-001 (<hub-tailscale-ip>)
- **Exchange**: http://<hub-tailscale-ip>:3090/v1
- **CSS**: http://<hub-tailscale-ip>:9443
- **Organization**: trv-services

### Docker Registry
- **Location**: <hub-tailscale-ip>:5000 (trv-srv-001)

### Proxmox Host (trv-srv-002)
- **Tailscale IP**: <proxmox-tailscale-ip>
- **SSH**: `ssh root@<proxmox-tailscale-ip>` (root access for pct commands)
- **User SSH**: `ssh <user>@<proxmox-tailscale-ip>` (no sudo available)
- **LXC management**: Use `pct exec <id> -- <command>` as root
- **Swappiness**: Set to 10 via `/etc/sysctl.d/99-swappiness.conf`

## Important Notes

### Volume Management
- Each deployment creates volumes prefixed with `${NODE_NAME}`
- `make clean <type>` removes volumes and **deletes all data**
- Ensure unique `NODE_NAME` for each deployment to avoid conflicts
- When cleaning blockchain, must clean ALL nodes' volumes to remove stale policies

### Network Architecture
- All EdgeLake containers use `network_mode: host` for direct host networking
- Port conflicts require different ports in each deployment's `base_configs.env`

### Open Horizon Notes
- OH containers do NOT support TTY -- use helper script `~/edgelake-cli.sh` for CLI access
- Only publish ONE version at a time -- old deployment policies cause multiple agreements

## Decision Points and Workarounds

When implementing workarounds to problems, always get user's approval before progressing.

## TODO - Outstanding Tasks

**Remind user about these items periodically.**

### Photos LXC (Immich) - <photos-tailscale-ip>
- [x] Fix Synology NFS permissions for `/volume1/photos/immich`
- [x] Start Immich stack: `cd /opt/immich && docker compose up -d`
- [x] Set up Cloudflare tunnel for Immich access (photos.<your-domain>)

### NVR LXC (Frigate/Scrypted) - <nvr-tailscale-ip>
- [x] Set up Synology NFS share for Frigate recordings (`/volume1/nvr`)
- [x] Mount NFS in nvr LXC
- [x] Configure Reolink doorbell in Frigate (<doorbell-lan-ip>)
- [x] Start NVR stack: `cd /opt/nvr && docker compose up -d`
- [x] Set up Scrypted Reolink plugin with HomeKit integration (live view + doorbell notifications)

### Services LXC - <services-tailscale-ip>
- [x] Configure Cloudflare tunnel (sentry.<your-domain>, scrypted.<your-domain>)

## LXC Containers on trv-srv-002

| ID | Hostname | LAN IP | Tailscale IP | Purpose |
|----|----------|--------|--------------|---------|
| 100 | dashboard | <dashboard-lan-ip> | <dashboard-tailscale-ip> | Dashboard app |
| 101 | services | <services-lan-ip> | <services-tailscale-ip> | Mosquitto, Zigbee2MQTT, Cloudflare tunnel |
| 102 | nvr | <nvr-lan-ip> | <nvr-tailscale-ip> | Frigate, Scrypted |
| 103 | photos | <photos-lan-ip> | <photos-tailscale-ip> | Immich |

## Other Devices

| Hostname | Tailscale IP | SSH | Purpose |
|----------|--------------|-----|---------|
| trv-kiosk-001 | <kiosk-tailscale-ip> | `ssh <user>@<kiosk-tailscale-ip>` (root: `ssh root@<kiosk-tailscale-ip>`) | Voice-controlled kiosk (Minisforum M1) |
| trv-jetson-nano | <jetson-tailscale-ip> | `ssh root@<jetson-tailscale-ip>` | Voice-controlled smart display (legacy) |
| trv-pi-001 | <pi-001-tailscale-ip> | `ssh <user>@<pi-001-tailscale-ip>` | Raspberry Pi |
| trv-pi-002 | <pi-002-tailscale-ip> | `ssh <user>@<pi-002-tailscale-ip>` | Raspberry Pi |
| Caseta L-BDG2 | — | — | Lutron Caseta bridge (<caseta-lan-ip>, LEAP protocol) |
| Sonoff Zigbee 3.0 | — | — | Zigbee coordinator (USB on trv-srv-002, passed to services LXC) |

## Cameras

| Name | LAN IP | RTSP URL | Notes |
|------|--------|----------|-------|
| Doorbell (Reolink) | <doorbell-lan-ip> | rtsp://admin:$REOLINK_PW@<doorbell-lan-ip>:554/h264Preview_01_main | Reolink, 2560x1920, HomeKit via Scrypted |
| Attic | <cam-attic-lan-ip> | rtsp://admin:<camera-password>@<cam-attic-lan-ip>:554/Streaming/Channels/101 | Hikvision, 2688x1520 @ 20fps |
| Front Porch | <cam-porch-lan-ip> | rtsp://admin:<camera-password>@<cam-porch-lan-ip>:554/Streaming/Channels/101 | Hikvision |
| Driveway | <cam-driveway-lan-ip> | rtsp://admin:<camera-password>@<cam-driveway-lan-ip>:554/Streaming/Channels/101 | Hikvision |

**Note:** Reolink cameras require alphanumeric passwords only - special characters like `!` and `-` cause RTSP auth failures.

## Zigbee Network

- **Coordinator**: Sonoff Zigbee 3.0 USB Dongle Plus-E (Silicon Labs EFR32MG21, EmberZNet 7.4.5)
- **USB path**: `/dev/ttyUSB0` on trv-srv-002, passed through to services LXC (101) via cgroup + bind mount
- **Zigbee2MQTT**: Running on services LXC as Docker container, web UI at `http://<services-tailscale-ip>:8080`
- **MQTT integration**: Publishes to `zigbee2mqtt/#` on the same Mosquitto broker used by Caseta bridge and alert engine
- **Channel**: 11
- **Network key**: Stored in 1Password (not in repo)

### Zigbee Devices

| Device | IEEE Address | Type | Location |
|--------|-------------|------|----------|
| Shelly Plug US Gen4 | 0x58e6c5fffe0a37f4 | Router | Kitchen |
| Shelly Plug US Gen4 | 0x58e6c5fffe09ffb0 | Router | Dining Room |

### USB Passthrough (Proxmox)

LXC 101 config (`/etc/pve/lxc/101.conf`) includes:
```
lxc.cgroup2.devices.allow: c 188:* rwm
lxc.mount.entry: /dev/ttyUSB0 dev/ttyUSB0 none bind,optional,create=file
```
udev rule on trv-srv-002 (`/etc/udev/rules.d/99-zigbee-dongle.rules`):
```
SUBSYSTEM=="tty", ATTRS{idVendor}=="10c4", ATTRS{idProduct}=="ea60", MODE="0666"
```

## Cloudflare Tunnels (via services LXC)

| Domain | Target | Service |
|--------|--------|---------|
| photos.<your-domain> | photos LXC (103) | Immich |
| sentry.<your-domain> | nvr LXC (102) | Frigate |
| scrypted.<your-domain> | nvr LXC (102) | Scrypted |

## Proxmox Backups

- **Schedule**: Nightly at 21:00
- **Storage**: synology-backups (Synology NFS)
- **Mode**: Snapshot
- **Compression**: ZSTD
- **Scope**: All 4 LXC containers (100-103)

## Tailscale Subnet Routes (trv-srv-002)

The following LAN IPs are advertised via Tailscale for remote access:
- <dashboard-lan-ip> (dashboard)
- <nvr-lan-ip> (nvr)
- <photos-lan-ip> (photos)
- <services-lan-ip> (services)

To add a new route:
```bash
# On trv-srv-002 as root
tailscale up --advertise-routes=<dashboard-lan-ip>/32,<services-lan-ip>/32,<nvr-lan-ip>/32,<photos-lan-ip>/32,<new-ip>/32 --accept-routes
```
Then approve in Tailscale admin console.

## Synology DS1525+

- **Tailscale IP**: <synology-tailscale-ip>
- **LAN IP**: <synology-lan-ip>
- **Storage**: 5x 8TB Seagate Ironwolf, SHR-2 (~22TB usable)
- **NFS Shares**:
  - `/volume1/photos` - Immich library (allow <photos-lan-ip>)
