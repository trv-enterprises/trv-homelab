# NVR Stack

Network Video Recorder setup running on LXC container on trv-srv-002 (Proxmox).

## Container Info

| Property | Value |
|----------|-------|
| LXC ID | 102 |
| Hostname | nvr |
| Tailscale IP | <nvr-tailscale-ip> |
| LAN IP | <nvr-lan-ip> |
| Location | trv-srv-002 (Proxmox) |

## Services

### Frigate

Object detection NVR with AI-powered motion detection.

| Property | Value |
|----------|-------|
| Web UI | http://<nvr-tailscale-ip>:5000 |
| RTSP | rtsp://<nvr-tailscale-ip>:8554 |
| WebRTC | http://<nvr-tailscale-ip>:8555 |
| Config | `/opt/nvr/frigate/config/config.yml` |
| Storage | `/mnt/nvr` (NFS from Synology <synology-lan-ip>:/volume1/nvr) |

### Scrypted

HomeKit integration and camera management.

| Property | Value |
|----------|-------|
| Web UI | https://<nvr-tailscale-ip>:10443 |
| Config | `/opt/nvr/scrypted/` |

### Mosquitto

MQTT broker for Frigate events.

| Property | Value |
|----------|-------|
| Port | 1883 |
| Config | `/opt/nvr/mosquitto/config/` |

## Commands

```bash
# SSH to container
ssh <user>@<nvr-tailscale-ip>

# Start/stop services
cd /opt/nvr
docker compose up -d
docker compose down
docker compose logs -f frigate

# Restart single service
docker compose restart frigate
```

## Adding a Camera to Frigate

1. Open Frigate UI: http://<nvr-tailscale-ip>:5000
2. Edit config at `/opt/nvr/frigate/config/config.yml`
3. Add camera under `cameras:` section:

```yaml
cameras:
  my_camera:
    enabled: true
    ffmpeg:
      inputs:
        - path: rtsp://user:pass@camera-ip:554/stream
          roles:
            - detect
            - record
    detect:
      width: 1920
      height: 1080
      fps: 5
    objects:
      track:
        - person
        - car
    record:
      enabled: true
      retain:
        days: 7
        mode: motion
```

4. Add to go2rtc for WebRTC streaming:

```yaml
go2rtc:
  streams:
    my_camera:
      - rtsp://user:pass@camera-ip:554/stream
```

5. Restart Frigate: `docker compose restart frigate`

## Camera RTSP URLs

Common RTSP URL formats:

| Brand | URL Format |
|-------|------------|
| LaView | `rtsp://admin:password@ip:554/main/av_stream` |
| Hikvision | `rtsp://user:pass@ip:554/Streaming/Channels/101` |
| Reolink | `rtsp://user:pass@ip:554/h264Preview_01_main` |
| Amcrest | `rtsp://user:pass@ip:554/cam/realmonitor?channel=1&subtype=0` |
| Generic ONVIF | `rtsp://user:pass@ip:554/stream1` |

Test RTSP stream with ffprobe:
```bash
ffprobe -rtsp_transport tcp rtsp://user:pass@ip:554/stream
```

## Viewing Camera Feeds

### Frigate Web UI
- Live view: http://<nvr-tailscale-ip>:5000
- Shows all cameras, events, and recordings
- Built-in playback for recorded clips

### RTSP Restream (via go2rtc)
Access any camera via Frigate's restream:
```
rtsp://<nvr-tailscale-ip>:8554/camera_name
```

### WebRTC (low latency)
For near-realtime viewing in browser via Frigate UI.

### VLC
```bash
vlc rtsp://<nvr-tailscale-ip>:8554/camera_name
```

### HomeKit (via Scrypted)
1. Open Scrypted: https://<nvr-tailscale-ip>:10443
2. Add Frigate plugin
3. Configure HomeKit bridge
4. Cameras appear in Apple Home app

## TODO

- [x] Mount Synology NFS share for recordings (`/volume1/nvr`)
- [x] Configure Reolink doorbell (<doorbell-lan-ip>) in Frigate
- [x] Set up Scrypted Reolink plugin with HomeKit (two-way audio, notifications)
- [ ] Add remaining cameras from Synology Surveillance Station

## Active Cameras

| Camera | IP | Frigate Name | Scrypted Plugin |
|--------|-----|--------------|-----------------|
| Reolink Doorbell | <doorbell-lan-ip> | doorbell | @scrypted/reolink |
| Front Porch | <cam-porch-lan-ip> | front_porch | Frigate Bridge |
| Driveway | <cam-driveway-lan-ip> | driveway | Frigate Bridge |
| Attic | <cam-attic-lan-ip> | attic | Frigate Bridge |
