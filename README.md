# trv-homelab

Infrastructure and deployment configurations for the TRV homelab network.

## Architecture

![TRV Homelab Architecture](docs/TRV%20Homelab%20Infrastructure.png)

## Network Map

| Device | Tailscale IP | Role | Services |
|--------|-------------|------|----------|
| trv-srv-001 | <hub-tailscale-ip> | Hub server | EL master, EL query, OH hub, ts-store, Docker registry |
| trv-srv-002 | <proxmox-tailscale-ip> | Proxmox host | LXC containers (dashboard, services, nvr, photos) |
| trv-srv-012 | <edge-srv-012-tailscale-ip> | Edge server | EL operator (docker-compose) |
| trv-srv-014 | <edge-srv-014-tailscale-ip> | Edge server | EL operator (Open Horizon) |
| trv-jetson-nano | <jetson-tailscale-ip> | Jetson Nano | Voice-controlled kiosk (Our Kiosk) |
| trv-pi-001 | <pi-001-tailscale-ip> | Raspberry Pi | ts-store, SenseHat |
| trv-pi-002 | <pi-002-tailscale-ip> | Raspberry Pi | ts-store, Mosquitto, Shelly |
| Synology DS1525+ | <synology-tailscale-ip> | NAS | NFS shares, Proxmox backup target |
| Sonoff Zigbee 3.0 | — | USB dongle | Zigbee coordinator (on trv-srv-002, passed to services LXC) |

## Repository Structure

```
trv-homelab/
├── hub/                    # Centralized infrastructure (trv-srv-001)
│   ├── edgelake/           # EL master + query nodes (docker-compose)
│   └── open-horizon/       # OH management hub configs
│
├── edge/                   # Deployable to many devices
│   ├── edgelake-operator/  # EL operator (4 deployment methods)
│   │   ├── docker-compose/ # Direct docker-compose
│   │   ├── open-horizon/   # OH-managed deployment
│   │   ├── helm-charts/    # Kubernetes Helm charts
│   │   └── kube-operator-go/  # Go Kubernetes operator
│   ├── zigbee2mqtt/        # Zigbee2MQTT (services LXC, Docker Compose)
│   ├── tsstore/            # ts-store generic deployment
│   └── mosquitto/          # MQTT broker deployment
│
├── devices/                # Per-device instance configs
│   ├── our-kiosk/          # Jetson Nano: voice-controlled smart display
│   ├── synology/           # Synology DS1525+ NAS
│   ├── trv-pi-001/         # Pi: ts-store + SenseHat
│   ├── trv-pi-002/         # Pi: Mosquitto broker
│   ├── trv-srv-001/        # Hub: ts-store stats
│   ├── trv-srv-012/        # Edge: EL operator (docker-compose)
│   └── trv-srv-014/        # Edge: EL operator (OH)
│
├── tools/                  # Development & testing tools
│   ├── mcp-client/         # Full-stack MCP chat app
│   ├── mcp-server/         # EL MCP server build/deploy
│   └── data-loader/        # Sample data loader
│
└── docs/                   # Architecture & reference docs
```

## Quick Reference

### EdgeLake Operations (Hub)

```bash
cd hub/edgelake
make up master          # Start master node
make up query           # Start query node
make status-all         # Check all hub nodes
make attach query       # Attach to query CLI
```

### EdgeLake Operations (Edge)

```bash
cd edge/edgelake-operator/docker-compose
make up operator        # Start operator node
make status operator    # Check operator status
```

### Open Horizon

```bash
cd hub/open-horizon
make -f Makefile.oh oh-publish-all    # Publish services
make -f Makefile.oh oh-list-services  # List services
```

### Zigbee2MQTT (Services LXC)

```bash
cd edge/zigbee2mqtt
make deploy             # Sync compose + config, restart stack
make status             # Show service status
make logs               # Follow zigbee2mqtt logs
make permit-join        # Enable Zigbee pairing mode
make stop-join          # Disable pairing mode
```

Web UI: `http://<services-tailscale-ip>:8080`

### MCP Server Build/Deploy

```bash
cd tools/mcp-server
make build arch=arm64   # Build for arm64
make deploy             # Deploy to target host
```

## Key Concepts

- **Hub** (`hub/`): Services that run centrally on trv-srv-001 -- the EdgeLake master/query nodes and the Open Horizon management hub
- **Edge** (`edge/`): Generic deployment configs that can target any device. Multiple deployment methods for EdgeLake operators
- **Devices** (`devices/`): Per-device instance configs. Thin -- mostly point to edge/ for the actual deployment configs
- **Tools** (`tools/`): Build tooling, test utilities, and development apps

## Related Repos

- [EdgeLake](https://github.com/EdgeLake/EdgeLake) -- EdgeLake/AnyLog source code
- [ts-store](https://github.com/<github-username>/ts-store) -- Time-series store binary
- EdgeLake documentation: `/path/to/documentation`
