# trv-homelab

Infrastructure and deployment configurations for the TRV homelab network.

## Network Map

| Device | Tailscale IP | Role | Services |
|--------|-------------|------|----------|
| trv-srv-002 | <proxmox-tailscale-ip> | Proxmox host | LXC containers (dashboard, services, nvr, photos) |
| trv-kiosk-001 | <kiosk-tailscale-ip> | Kiosk | Voice-controlled smart display |
| trv-pi-001 | <pi-001-tailscale-ip> | Raspberry Pi | ts-store, SenseHat |
| trv-pi-002 | <pi-002-tailscale-ip> | Raspberry Pi | ts-store, Mosquitto, Shelly |
| Synology DS1525+ | <synology-tailscale-ip> | NAS | NFS shares, Proxmox backup target |
| Sonoff Zigbee 3.0 | — | USB dongle | Zigbee coordinator (on trv-srv-002, passed to services LXC) |

## Repository Structure

```
trv-homelab/
├── edge/                   # Deployable service configs
│   ├── zigbee2mqtt/        # Zigbee2MQTT (services LXC, Docker Compose)
│   ├── homebridge/         # Homebridge + mqttthing (HomeKit bridge)
│   ├── weather-poller/     # Weather data poller (Go, Visual Crossing API)
│   ├── sensor-alert-engine/# Alert rules engine for sensor thresholds
│   ├── caseta-bridge/      # Lutron Caseta MQTT bridge
│   ├── mosquitto/          # MQTT broker deployment
│   ├── tsstore/            # ts-store generic deployment
│   └── shelly/             # Shelly device integration
│
├── devices/                # Per-device instance configs
│   ├── nvr/                # Frigate + Scrypted (NVR LXC)
│   ├── synology/           # Synology DS1525+ NAS
│   ├── trv-pi-001/         # Pi: ts-store + SenseHat
│   ├── trv-pi-002/         # Pi: Mosquitto broker
│   └── trv-srv-001/        # Hub: ts-store stats
│
├── tools/                  # Development & operations tools
│   ├── ansible/            # Ansible playbooks & roles for deployment
│   ├── svr-scan/           # Server inventory scanner
│   └── web-launcher/       # Local web dashboard launcher
│
└── docs/                   # Architecture & reference docs
```

## Quick Reference

### Zigbee2MQTT (Services LXC)

```bash
cd edge/zigbee2mqtt
make deploy             # Sync compose + config, restart stack
make status             # Show service status
make logs               # Follow zigbee2mqtt logs
make permit-join        # Enable Zigbee pairing mode
make stop-join          # Disable pairing mode
```

### Ansible Deployments

```bash
cd tools/ansible
ansible-playbook -i <inventory> playbooks/dashboard-deploy.yml
ansible-playbook -i <inventory> playbooks/weather-poller-deploy.yml
ansible-playbook -i <inventory> playbooks/server-report.yml
```

### Homebridge

```bash
cd edge/homebridge
make deploy             # Deploy config + codec to Homebridge container
```

## Key Concepts

- **Edge** (`edge/`): Generic deployment configs for services that run on infrastructure nodes (Zigbee2MQTT, Homebridge, weather poller, MQTT broker, etc.)
- **Devices** (`devices/`): Per-device instance configs — systemd services, scripts, and device-specific setup
- **Tools** (`tools/`): Ansible playbooks for automated deployment, server scanning, and local development utilities

## Related Repos

- [trv-kiosk](https://github.com/trv-enterprises/trv-kiosk) — Voice-controlled smart display (React + Python)
- [ts-store](https://github.com/<github-username>/ts-store) — Time-series store binary
