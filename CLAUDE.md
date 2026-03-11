# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This repository consolidates homelab infrastructure: Zigbee/IoT device management, MQTT broker configs, NVR/camera setup, Ansible deployment playbooks, and development tools.

## Directory Layout

- `edge/` -- Generic deployment configs, deployable to any device
  - `edge/zigbee2mqtt/` -- Zigbee2MQTT deployment (services LXC, Docker Compose)
  - `edge/homebridge/` -- Homebridge + mqttthing (HomeKit bridge via MQTT)
  - `edge/weather-poller/` -- Go service polling Visual Crossing API, publishes to MQTT
  - `edge/sensor-alert-engine/` -- Go alert engine for sensor threshold rules
  - `edge/caseta-bridge/` -- Lutron Caseta MQTT bridge
  - `edge/mosquitto/` -- MQTT broker deployment
  - `edge/tsstore/` -- ts-store deployment configs
  - `edge/shelly/` -- Shelly device integration
- `devices/` -- Per-device instance configs (thin, reference edge/ configs)
- `tools/` -- Development and testing utilities
  - `tools/ansible/` -- Ansible playbooks and roles for deployment
  - `tools/svr-scan/` -- Server inventory scanner
  - `tools/web-launcher/` -- Local web dashboard launcher
- `docs/` -- Architecture docs and diagrams

## Commands

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

### Homebridge

```bash
# From edge/homebridge/
make deploy              # Deploy config + codec to Homebridge container
```

### Ansible Deployments

```bash
# From tools/ansible/ (use -i to point to your inventory)
ansible-playbook -i <inventory> playbooks/dashboard-deploy.yml
ansible-playbook -i <inventory> playbooks/weather-poller-deploy.yml
ansible-playbook -i <inventory> playbooks/server-report.yml
ansible-playbook -i <inventory> playbooks/tsstore-deploy.yml
```

## Proxmox Host (trv-srv-002)

- **SSH**: `ssh root@<proxmox-tailscale-ip>` (root access for pct commands)
- **LXC management**: Use `pct exec <id> -- <command>` as root

## LXC Containers on trv-srv-002

| ID | Hostname | Purpose |
|----|----------|---------|
| 100 | dashboard | Dashboard app |
| 101 | services | Mosquitto, Zigbee2MQTT, Homebridge, Cloudflare tunnel |
| 102 | nvr | Frigate, Scrypted |
| 103 | photos | Immich |

## Zigbee Network

- **Coordinator**: Sonoff Zigbee 3.0 USB Dongle Plus-E (Silicon Labs EFR32MG21, EmberZNet 7.4.5)
- **USB path**: `/dev/ttyUSB0` on trv-srv-002, passed through to services LXC (101) via cgroup + bind mount
- **Zigbee2MQTT**: Running on services LXC as Docker container
- **MQTT integration**: Publishes to `zigbee2mqtt/#` on the same Mosquitto broker used by Caseta bridge and alert engine
- **Channel**: 11
- **Network key**: Stored in 1Password (not in repo)

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

## Decision Points and Workarounds

When implementing workarounds to problems, always get user's approval before progressing.

## Related Repos

- [trv-kiosk](https://github.com/trv-enterprises/trv-kiosk) -- Voice-controlled smart display (React + Python)
