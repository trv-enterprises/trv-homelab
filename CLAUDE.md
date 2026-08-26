# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This repository consolidates homelab infrastructure: Zigbee/IoT device management, MQTT broker configs, NVR/camera setup, Ansible deployment playbooks, and development tools.

This repo is **public and sanitized**. Real Tailscale IPs, inventory, host_vars
and vault secrets live in the private `homelab-deploy` overlay, which supplies
them at deploy time. Write `<name-tailscale-ip>` placeholders here, never a real
tailnet address, key, or secret. RFC1918 LAN addresses are acceptable.

## Directory Layout

- `edge/` -- Generic deployment configs, deployable to any device
  - `edge/zigbee2mqtt/` -- Zigbee2MQTT deployment (services LXC, Docker Compose)
  - `edge/homebridge/` -- Homebridge + mqttthing (HomeKit bridge via MQTT)
  - `edge/weather-poller/` -- Go service polling Visual Crossing API, publishes to MQTT
  - `edge/caseta-bridge/` -- Lutron Caseta MQTT bridge
  - `edge/mosquitto/` -- MQTT broker deployment
  - `edge/tsstore/` -- ts-store deployment configs
  - `edge/shelly/` -- Shelly device integration (incl. MQTT collector for sleepy H&T sensors)
  - `edge/synology-snmp/` -- Synology NAS SNMP collector: vendor MIBs + OID -> ts-store field map
- `devices/` -- Per-device instance configs (thin, reference edge/ configs)
- `tools/` -- Development and testing utilities
  - `tools/ansible/` -- Ansible playbooks and roles for deployment
  - `tools/svr-scan/` -- Server inventory scanner
  - `tools/web-launcher/` -- Local web dashboard launcher
- `docs/` -- Architecture docs and diagrams

## Architecture Docs

Read these before changing the subsystems they describe -- each records *why*
the design is what it is, which is not recoverable from the code:

- `docs/nightlight-automation.md` -- motion nightlight: ownership arbitration
  between the automation engine, manual override, and the device's own on-board
  rule. Explains the `0xFC00` private cluster and the self-override hazard.
- `docs/nvr-architecture.md` -- camera split across Surveillance Station,
  Frigate, and Scrypted.
- `docs/IoT Component Diagram.drawio` -- component overview (PNG alongside).

## Go Services

One Go module lives here, building to a container published on GHCR under the
`trv-enterprises` org. (Marshal, the former alert engine, moved to its own repo
— see Related Repos.)

| Service | Module path | Purpose |
|---|---|---|
| `edge/weather-poller/` | `github.com/trv-enterprises/trv-homelab/edge/weather-poller` | Visual Crossing -> MQTT |

It uses the path-based module name, so the import path matches where it
actually lives. Keep it that way when adding another.

### Marshal (moved out)

The alert/automation engine that drives the nightlight now lives in
**`trv-enterprises/trv-marshal`**, renamed Marshal — "alert engine" stopped
describing it once it gained edge-triggered actuation.

What stays here is the **deployment** side only: the `marshal` Ansible role,
`marshal-deploy.yml`, and its service block in the services-stack compose
template. The code, its CI and its releases are in that repo.

Its paho MQTT reconnect notes moved with it (`trv-marshal/CLAUDE.md`), and the
library-level version is in the global `paho-mqtt-go-pitfalls` memory.

One thing deliberately did NOT move: the alert payload's `source` field is
still `"alert_engine"`. It is wire format that trv-kiosk matches on — renaming
it silently breaks alert display there.

## CI

GitHub Actions, in `.github/workflows/`:

- **`pr-checks.yml`** — runs on every PR into main (and pushes to main).
  Per-module `go build` / `go vet` / `go test -race`, `gofmt` scoped to the
  files the PR touches, `govulncheck`, plus `yamllint` and
  `ansible-playbook --syntax-check` over the Ansible tree.
- **`publish-containers.yml`** — builds and pushes a service image to GHCR,
  multi-arch (amd64 + arm64), gated on that service's tests passing.

**Tags carry the service name**, because a bare `v*` cannot say which of the
two images it means:

```bash
git tag -a weather-poller/v0.1.0 -m "..." && git push origin weather-poller/v0.1.0
# -> ghcr.io/trv-enterprises/weather-poller:0.1.0  (+ :latest when not a prerelease)
```

A hyphenated version is treated as a prerelease and does **not** move
`:latest` — the deploy roles default to `latest`, so an rc must not become
what an unpinned host picks up. Existing bare `v0.2.0-rc.*` tags remain valid
history but no longer trigger anything.

`caseta-bridge` has a Dockerfile but is deliberately not published: it deploys
as Python over rsync + systemd, and that Dockerfile is vestigial.

**A GHCR package needs the repo granted Actions access before CI can push to
it.** Both existing packages predated this workflow — they were created by
laptop pushes with a PAT, so they carried no repository link and `GITHUB_TOKEN`
inherited nothing, giving `denied: permission_denied: write_package`. There is
no API for this (checked REST and GraphQL): it is Package settings -> Manage
Actions access -> add the repo with role **Write**. Note that is a SEPARATE
control from "Repository source" — setting the source alone does not grant the
push. Images this workflow builds carry `org.opencontainers.image.source`, so
any *new* package it creates links itself.

**Keep each Dockerfile's Go base image in step with its `go.mod`.** They are
linked by nothing but attention, and a `go.mod` bump past the base image fails
the build at `go mod download`.

Go jobs resolve their toolchain from the module's own `go.mod` rather than a
pinned version — `go build ./...` at the repo root builds nothing, since the
module lives under `edge/`.

`.yamllint` is deliberately permissive — it fails on parse errors, duplicate
keys, tabs and implicit octal (a real Ansible file-mode footgun) but not on
cosmetic style, so it does not red-light 48 pre-existing files.

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

Playbooks live here; the inventory that feeds them lives in `homelab-deploy`,
which wraps each of these in a `make` target. Run them from there unless you are
testing against a different inventory.

```bash
# From tools/ansible/ (use -i to point to your inventory)
ansible-playbook -i <inventory> playbooks/marshal-deploy.yml
ansible-playbook -i <inventory> playbooks/dashboard-deploy.yml
ansible-playbook -i <inventory> playbooks/dashboard-preprod-deploy.yml
ansible-playbook -i <inventory> playbooks/docker-stats-deploy.yml
ansible-playbook -i <inventory> playbooks/nut-client-deploy.yml
ansible-playbook -i <inventory> playbooks/our-kiosk-deploy.yml
ansible-playbook -i <inventory> playbooks/our-kiosk-setup.yml
ansible-playbook -i <inventory> playbooks/our-kiosk-setup-minisforum.yml
ansible-playbook -i <inventory> playbooks/server-report.yml
ansible-playbook -i <inventory> playbooks/simulators-deploy.yml
ansible-playbook -i <inventory> playbooks/synology-snmp-deploy.yml
ansible-playbook -i <inventory> playbooks/tsstore-deploy.yml
ansible-playbook -i <inventory> playbooks/weather-poller-deploy.yml
```

Roles: `marshal`, `dashboard`, `nut-client`, `server-report`,
`services-stack`, `simulators`, `tsstore`, `voice-display`, `weather-poller`.

`services-stack` owns the shared `docker-compose.yml` for the services LXC.
Every services-LXC playbook depends on it, and individual service roles must not
ship their own copy of that compose file.

## ts-store Integration

Several collectors here write to ts-store. Two constraints bite repeatedly:

- **Stores are not auto-created.** ts-store does not create a store on first
  write -- the write fails on the missing meta file. The `tsstore` role creates
  each store explicitly and applies its schema before the collector starts.
- **Schema stores project every record through the *current* schema, by field
  index.** There is one active schema and no per-record versioning. Appending a
  field is safe (older records simply lack the key). Dropping one is
  read-destructive. Renaming, retyping, or reusing an index is rejected outright
  and requires recreating the store, which loses history. Treat any non-append
  schema change as breaking and coordinate with consumers first.

Collector store specs live in the `tsstore` role's `vars/main.yml`, overridable
per host from the deployment inventory.

`edge/synology-snmp/oid-map.yml` is the OID -> field map for the NAS collector.
Its header documents the provenance rules (vendor MIB vs. 2025 MIB Guide vs.
live snmpwalk) and the table-shape convention: SNMP table walks become *tall*
rows keyed by a discriminator, scalars become one *wide* row per poll. ts-store
schema stores hold flat typed fields only -- no embedded arrays.

## Proxmox Host (trv-srv-002)

- **SSH**: `ssh root@<proxmox-tailscale-ip>` (root access for pct commands)
- **LXC management**: Use `pct exec <id> -- <command>` as root

## LXC Containers on trv-srv-002

| ID | Hostname | Purpose |
|----|----------|---------|
| 100 | dashboard | Dashboard app |
| 101 | services | Mosquitto, Zigbee2MQTT, Homebridge, Caseta bridge, weather-poller, Marshal, Cloudflare tunnel |
| 102 | nvr | Frigate, Scrypted |
| 103 | photos | Immich |

## Zigbee Network

- **Coordinator**: Sonoff Zigbee 3.0 USB Dongle Plus-E (Silicon Labs EFR32MG21, EmberZNet 7.4.5)
- **USB path**: `/dev/ttyUSB0` on trv-srv-002, passed through to services LXC (101) via cgroup + bind mount
- **Zigbee2MQTT**: Running on services LXC as Docker container
- **MQTT integration**: Publishes to `zigbee2mqtt/#` on the same Mosquitto broker used by Caseta bridge and Marshal
- **Channel**: 11
- **Network key**: Stored in 1Password (not in repo)

### Vendor private clusters

Some devices expose behavior only through manufacturer-private clusters that
Zigbee2MQTT defines but wires no converter to -- unreachable via `<device>/set`,
writable only through the raw ZCL API. The Third Reality nightlight's on-board
motion rule (`0xFC00`, manufacturer `0x130D`) is the worked example.

**These clusters commonly do not answer reads.** When configuration cannot be
queried back, the deployment inventory is the source of truth and the Ansible
role re-asserts it on every deploy, which also repairs drift after a power cycle
or OTA. See `docs/nightlight-automation.md`.

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

## MQTT Conventions

Mosquitto on the services LXC is the single broker for all consumers. The NVR
broker bridges selected topics to it rather than being consumed directly.

| Topic pattern | Direction | Purpose |
|---|---|---|
| `zigbee2mqtt/<device>` | sensor -> broker | Zigbee device state |
| `zigbee2mqtt/<device>/set` | broker -> device | Zigbee device commands |
| `caseta/<device>` | bridge -> broker | Caseta device state |
| `caseta/<device>/set` | broker -> bridge | Caseta device commands |
| `sensors/alerts` | Marshal -> broker | Alert events |
| `weather/*` | poller -> broker | Current conditions, forecasts, alerts |
| `frigate/reviews` | NVR bridge -> broker | Detection events |
| `automation/<name>/enable` | any -> engine | Park/resume an automation rule |
| `automation/<name>/owner` | engine -> broker | Retained current owner (bare string) |

Mosquitto persistence requires a **bind-mounted config file**, not a named
volume -- persistence was silently broken until this was corrected.

Retained `automation/*/owner` payloads are bare strings, not Z2M JSON. Consumers
that assume JSON (e.g. the Homebridge codec) must handle that before parsing.

## Decision Points and Workarounds

When implementing workarounds to problems, always get user's approval before progressing.

## Documentation Boundary

Knowledge follows the same boundary as code. Service behavior, protocol
semantics, gotchas, and architecture belong **here**, next to what they
describe. Real IPs, inventory, live version pins, and deploy targets belong in
the private `homelab-deploy` overlay.

The test: *would this still be true if the homelab were rebuilt on different
hardware with different IPs?* If yes, it belongs here. Prefer a nested
`CLAUDE.md` when the knowledge is specific to one service.

## Related Repos

- [trv-kiosk](https://github.com/trv-enterprises/trv-kiosk) -- Voice-controlled smart display (React + Python)
- [trv-marshal](https://github.com/trv-enterprises/trv-marshal) -- Marshal, the MQTT automation engine (split out of this repo 2026-08-26)
- `homelab-deploy` (private) -- real inventory, vault, host_vars, `make` deploy targets
- `trv-outpost-sim` -- simulator services, synced into the `simulators` role at deploy time
- `ts-store` -- the time-series store these collectors write to
