# Synology DS1525+

Network Attached Storage for photos, video recordings, and backups.

## Device Info

| Property | Value |
|----------|-------|
| Model | Synology DS1525+ |
| Tailscale IP | <synology-tailscale-ip> |
| LAN IP | <synology-lan-ip> |
| CPU | AMD Ryzen V1500B |
| RAM | 8GB |
| Network | 2x 2.5GbE |

## Storage

| Property | Value |
|----------|-------|
| Drives | 5x 8TB Seagate Ironwolf |
| RAID | SHR-2 (double fault tolerance) |
| Usable Capacity | ~22TB |

## Access

| Service | URL |
|---------|-----|
| DSM Web UI | http://<synology-lan-ip>:5000 or https://<synology-lan-ip>:5001 |
| File Station | http://<synology-lan-ip>:5000 (via DSM) |
| Surveillance Station | http://<synology-lan-ip>:9900 |

## NFS Shares

| Share | Path | Used By | Allowed Hosts |
|-------|------|---------|---------------|
| photos | `/volume1/photos` | Immich (LXC 103) | <photos-lan-ip> |
| nvr | `/volume1/nvr` | Frigate (LXC 102) | <nvr-nfs-lan-ip> (TODO) |

### NFS Mount Commands

From a client (e.g., LXC container):
```bash
# Mount photos share
mount -t nfs <synology-lan-ip>:/volume1/photos /mnt/photos

# Mount nvr share
mount -t nfs <synology-lan-ip>:/volume1/nvr /mnt/nvr

# Add to /etc/fstab for persistent mount
<synology-lan-ip>:/volume1/photos /mnt/photos nfs defaults 0 0
```

## Surveillance Station

Built-in NVR software for IP cameras.

| Property | Value |
|----------|-------|
| Web UI | http://<synology-lan-ip>:9900 |
| Included Licenses | 2 |
| Additional Needed | 2 (for 4 cameras total) |

### Camera Setup

1. Open Surveillance Station: http://<synology-lan-ip>:9900
2. IP Camera → Add → Auto-detect or manual
3. Enter camera credentials
4. Configure recording schedule and motion detection

### RTSP Restream

Surveillance Station can restream cameras via RTSP:
```
rtsp://user:pass@<synology-lan-ip>:554/Srehab01  # Camera 1
rtsp://user:pass@<synology-lan-ip>:554/Srehab02  # Camera 2
```

## Packages Installed

- [ ] Surveillance Station (TODO)
- Tailscale

## Backup Strategy

| Data | Location | Retention |
|------|----------|-----------|
| Photos | `/volume1/photos` | Permanent |
| NVR Recordings | `/volume1/nvr` | 30 days motion, 7 days continuous |
| Immich DB Backup | `/volume1/photos/immich-backup` | TODO |

## TODO

- [ ] Install Surveillance Station package
- [ ] Add 4 existing cameras to Surveillance Station
- [ ] Configure motion detection per camera
- [ ] Create NFS share `/volume1/nvr` for Frigate
- [ ] Set up Immich PostgreSQL backup cron job
- [ ] Purchase 2 additional camera licenses
