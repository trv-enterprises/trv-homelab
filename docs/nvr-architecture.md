# NVR Architecture

## Overview

The NVR (Network Video Recorder) system manages security cameras across the homelab using a split architecture:
- **Synology Surveillance Station** handles the 4 existing cameras with built-in motion detection
- **Frigate** provides AI-powered detection for the doorbell camera
- **Scrypted** bridges everything to Apple HomeKit

## Components

### Hardware

| Device | Model | Role |
|--------|-------|------|
| NAS | Synology DS1525+ | Storage, Surveillance Station |
| Proxmox Host | trv-srv-002 (Ryzen 9 PRO 8945HS) | Frigate/Scrypted LXC |
| Cameras (4x) | 3MP existing cameras | General surveillance |
| Doorbell | TBD | AI-detected entry point |

### Storage

- **Synology DS1525+**: 5x 8TB Seagate Ironwolf
- **RAID**: SHR-2 (dual fault tolerance)
- **Usable Capacity**: ~22TB formatted
- **NFS Share**: `/volume1/nvr` for Frigate recordings

### Network

| Service | Host | Tailscale IP | Ports |
|---------|------|--------------|-------|
| Frigate | nvr LXC | <nvr-tailscale-ip> | 5000 (UI), 8554 (RTSP), 8555 (WebRTC) |
| Scrypted | nvr LXC | <nvr-tailscale-ip> | 10443 (UI) |
| MQTT (nvr) | nvr LXC | <nvr-tailscale-ip> | 1883 |
| Surveillance Station | Synology | TBD | 5000 (DSM), 9900 (SS) |

## Architecture Diagram

```
┌─ 4 Existing Cameras ──────────────────────┐
│   RTSP streams                             │
│      ↓                                     │
│   Synology Surveillance Station            │
│      ├─ Motion detection (built-in)        │
│      ├─ Recording → local disk (SHR-2)     │
│      └─ RTSP restream                      │
│            ↓                               │
└────────────┬──────────────────────────────┘
             │
             ▼
┌─ Scrypted (nvr LXC) ──────────────────────┐
│   ├─ Surveillance Station plugin           │
│   │    └─ Playback, motion events          │
│   ├─ Frigate plugin (doorbell)             │
│   └─ HomeKit bridge → Apple Home           │
└───────────────────────────────────────────┘

┌─ Doorbell ────────────────────────────────┐
│   RTSP stream                              │
│      ↓                                     │
│   Frigate (nvr LXC)                        │
│      ├─ AI detection (CPU - no Coral)      │
│      │    └─ person, car, package          │
│      ├─ Recording → Synology NFS           │
│      ├─ MQTT events → local mosquitto      │
│      └─ go2rtc restream                    │
└───────────────────────────────────────────┘
```

## LXC Container: nvr

- **Proxmox ID**: 102
- **Hostname**: nvr
- **Tailscale IP**: <nvr-tailscale-ip>
- **Resources**: 4 cores, 4GB RAM, 32GB disk
- **OS**: Ubuntu 24.04

### Docker Services

| Container | Image | Purpose |
|-----------|-------|---------|
| frigate | ghcr.io/blakeblackshear/frigate:stable | AI detection, recording |
| scrypted | ghcr.io/koush/scrypted:latest | HomeKit bridge |
| mosquitto | eclipse-mosquitto:2 | MQTT for Frigate events |

### Directory Structure

```
/opt/nvr/
├── docker-compose.yml
├── frigate/
│   ├── config/
│   │   └── config.yml
│   └── storage/           # Local cache (recordings on NFS)
├── scrypted/
└── mosquitto/
    ├── config/
    │   └── mosquitto.conf
    ├── data/
    └── log/
```

## Setup Checklist

### Synology DS1525+

- [ ] Initial DSM setup
- [ ] Create storage pool (SHR-2)
- [ ] Create volume
- [ ] Install Surveillance Station package
- [ ] Purchase additional camera licenses (2 included, need 2 more for 4 cameras)
- [ ] Add 4 existing cameras to Surveillance Station
- [ ] Configure motion detection per camera
- [ ] Create NFS share for Frigate: `/volume1/nvr`
- [ ] Set NFS permissions for nvr LXC (<nvr-tailscale-ip>)

### nvr LXC (Proxmox)

- [x] Create LXC container (ID 102)
- [x] Configure Tailscale (<nvr-tailscale-ip>)
- [x] Install Docker
- [x] Create docker-compose.yml
- [x] Create Frigate config
- [ ] Mount Synology NFS share at `/mnt/synology/nvr`
- [ ] Update docker-compose.yml to use NFS mount
- [ ] Configure doorbell camera in Frigate
- [ ] Start services: `docker compose up -d`

### Scrypted

- [ ] Access UI: http://<nvr-tailscale-ip>:10443
- [ ] Install Synology Surveillance Station plugin
- [ ] Configure SS connection
- [ ] Install Frigate plugin
- [ ] Configure Frigate connection
- [ ] Install HomeKit plugin
- [ ] Pair with Apple Home

### Frigate

- [ ] Access UI: http://<nvr-tailscale-ip>:5000
- [ ] Update doorbell camera RTSP URL in config.yml
- [ ] Enable doorbell camera
- [ ] Verify AI detection working
- [ ] Configure detection zones

## Recording Retention

### Surveillance Station (4 cameras)

- Motion-triggered recording
- Retention configured per camera in SS

### Frigate (doorbell)

| Type | Retention |
|------|-----------|
| Motion clips | 30 days |
| AI events (person/car/package) | 60 days |

### Storage Estimates

With motion-only recording on 5 cameras (3MP):
- ~5-20GB per camera per day (activity dependent)
- ~25-100GB/day total
- 22TB usable = 7-24 months retention

## Integration Points

### MQTT Events

Frigate publishes events to local MQTT (mosquitto on nvr LXC):
- `frigate/events` - AI detection events
- `frigate/+/motion` - Motion events per camera

These can be bridged to the main Mosquitto instance on services LXC (<services-tailscale-ip>) if needed for ts-store or other applications.

### HomeKit

Scrypted exposes all cameras to Apple Home:
- Live view
- Motion notifications (from SS and Frigate)
- Recording playback (via Scrypted plugins)

## Maintenance

### Viewing Logs

```bash
ssh <user>@<nvr-tailscale-ip>
cd /opt/nvr
docker compose logs -f frigate
docker compose logs -f scrypted
```

### Updating Services

```bash
ssh <user>@<nvr-tailscale-ip>
cd /opt/nvr
docker compose pull
docker compose up -d
```

### Backup

- Frigate config: `/opt/nvr/frigate/config/config.yml`
- Scrypted data: `/opt/nvr/scrypted/`
- Recordings: Synology NFS (protected by SHR-2)
