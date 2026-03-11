# Scrypted + HomeKit Integration

This document summarizes the setup and findings from integrating Frigate with Apple HomeKit via Scrypted.

## Architecture

```
Camera (RTSP) → Frigate (detection) → MQTT → Scrypted (Frigate Bridge) → HomeKit → iPhone
```

## Components

| Component | Version | Purpose |
|-----------|---------|---------|
| Frigate | stable | Object detection NVR |
| Scrypted | 0.143.0 | HomeKit bridge |
| Frigate Bridge Plugin | @apocaliss92/scrypted-frigate-bridge 0.1.17 | Bridges Frigate events to Scrypted |
| Mosquitto | latest | MQTT broker for Frigate events |
| HomeKit Plugin | @scrypted/homekit 1.2.65 | Exposes cameras to Apple Home |

## Key Findings

### MQTT Configuration

The Frigate Bridge plugin requires MQTT credentials even if Mosquitto has `allow_anonymous true`:

**MQTT Plugin Settings:**
- External Broker: `mqtt://<nvr-lan-ip>`
- Username: `scrypted` (any value works)
- Password: `scrypted` (any value works)

Without credentials, the plugin shows:
```
[ERROR]: MQTT params not provided: {"mqttHost":"mqtt://<nvr-lan-ip>","mqttUsename":"NOT_SET","mqttPassword":"NOT_SET"}
```

### HomeKit Standalone Accessory Mode (Required)

**Standalone Accessory Mode is required for motion notifications to work.** Without it, the motion sensor may not register properly with HomeKit.

1. In Scrypted, go to camera → HOMEKIT section
2. Enable "Standalone Accessory Mode"
3. Restart HomeKit plugin
4. Re-pair camera in Home app using new pairing code

This creates a fresh HomeKit registration with proper motion sensor capabilities. Each camera will have its own pairing code.

### Motion Notification Settings

HomeKit notification filtering:
- **"Any Motion"** - Works with Scrypted/Frigate Bridge
- **"Specific Motion (People/Animals)"** - Does NOT work

The Frigate Bridge plugin sends `motionDetected: true/false` but doesn't include object classification labels that HomeKit needs for filtered notifications.

**Workaround:** Use "Any Motion" in HomeKit and let Frigate's zone/label filtering handle the specificity.

### Camera Extension Setup

Required extensions on each camera in Scrypted:

| Extension | Status | Purpose |
|-----------|--------|---------|
| Rebroadcast Plugin | ON | Stream handling |
| WebRTC Plugin | ON | Low-latency streaming |
| HomeKit | ON | HomeKit exposure |
| Frigate Motion Detector | ON | Motion events from Frigate |
| Snapshot Plugin | ON | Thumbnail generation |
| Frigate Object Detector | ON | Object detection events |

### Frigate Motion Detector Settings

- **Frigate camera name**: Must match exactly (e.g., `front_porch`)
- **Motion timeout**: 15 seconds
- **Report motion only on detection**: ON (requires Frigate Object Detector)

### Frigate Object Detector Settings

- **Labels to import**: `car`, `person`
- **Event types**: `new`, `update`
- **Zones**: Specify zone name (e.g., `Porch`)
- **Bounding box extension**: 10%

## Frigate Review Configuration

### Alerts vs Detections

```yaml
review:
  alerts:
    labels:
      - person
      - car
    required_zones:
      - Driveway
  detections:
    required_zones:
      - Street
```

- **Alerts**: High-priority events (person/car in specified zones)
- **Detections**: Lower-priority events (any object in specified zones)
- **Outside zones**: Motion only, no review item

### Zone Configuration

Zones are defined per-camera in Frigate config:

```yaml
zones:
  Porch:
    coordinates: 0.262,0.92,0.32,1,...
    loitering_time: 0
    inertia: 1
```

The zone name in Scrypted must **exactly match** the zone name in Frigate.

### Empty Labels Array

Setting `labels: []` disables that category entirely:
```yaml
review:
  alerts:
    labels: []  # No alerts will be created
```

## Troubleshooting

### No motion events in Scrypted

1. Check MQTT connection in Frigate Bridge plugin log
2. Verify MQTT credentials are set (even dummy values)
3. Test MQTT with: `mosquitto_sub -t 'frigate/#' -v`

### Motion events received but no HomeKit notification

1. Check camera Events tab in Scrypted for `motionDetected: true`
2. Verify HomeKit extension is enabled
3. Try Standalone Accessory Mode
4. Set HomeKit notifications to "Any Motion" (not filtered)

### Swap pressure on NVR container

Set swappiness at host level (trv-srv-002):
```bash
sysctl vm.swappiness=10
echo 'vm.swappiness=10' >> /etc/sysctl.d/99-swappiness.conf
```

LXC containers inherit host swappiness. Cgroup v2 doesn't support per-container swappiness.

### Clean up Docker storage

```bash
pct exec 102 -- docker system prune -a
```

## Useful Commands

### View MQTT events
```bash
ssh root@<proxmox-tailscale-ip> "pct exec 102 -- docker exec mosquitto mosquitto_sub -t 'frigate/#' -v"
```

### Check Frigate config
```bash
ssh root@<proxmox-tailscale-ip> "pct exec 102 -- docker exec frigate cat /config/config.yml"
```

### Restart services
```bash
ssh root@<proxmox-tailscale-ip> "pct exec 102 -- docker restart frigate"
ssh root@<proxmox-tailscale-ip> "pct exec 102 -- docker restart scrypted"
```

### Check container resources
```bash
ssh root@<proxmox-tailscale-ip> "pct exec 102 -- docker stats --no-stream"
ssh root@<proxmox-tailscale-ip> "pct exec 102 -- free -h"
```

## Network Details

| Service | LAN IP | Tailscale IP | Port |
|---------|--------|--------------|------|
| Frigate | <nvr-lan-ip> | <nvr-tailscale-ip> | 5000 |
| Scrypted | <nvr-lan-ip> | <nvr-tailscale-ip> | 10443 |
| Mosquitto | <nvr-lan-ip> | <nvr-tailscale-ip> | 1883 |

HomeKit Hub: Apple TV at <appletv-lan-ip>

## Reolink Doorbell Setup

For Reolink doorbells, use the **Reolink Plugin** instead of Frigate Bridge. This provides native support for doorbell-specific features.

### Why Reolink Plugin vs Frigate

| Feature          | Reolink Plugin           | Frigate Bridge       |
|------------------|--------------------------|----------------------|
| Two-way audio    | Yes                      | No                   |
| Doorbell button  | Yes                      | No                   |
| Live view        | Yes                      | Yes                  |
| Object detection | Basic (built-in)         | Advanced (Frigate AI)|
| Recording        | Scrypted NVR or camera SD| Frigate              |

For doorbells, the native features (button press, two-way talk) are more important than advanced object detection.

### Architecture

```
Reolink Doorbell → Scrypted (Reolink Plugin) → HomeKit → iPhone
```

### Installation

1. Install **@scrypted/reolink** plugin from Scrypted plugins (official plugin)
2. Add doorbell by IP address: `<doorbell-lan-ip>`
3. Username: `admin`, Password: stored in `$REOLINK_PW` env var on NVR

**Important:** Reolink passwords must be alphanumeric only. Special characters like `!`, `-`, `@` cause RTSP authentication failures even though the Reolink app accepts them.

### Scrypted Extensions

| Extension          | Status | Purpose               |
|--------------------|--------|-----------------------|
| Reolink Plugin     | ON     | Native camera control |
| Rebroadcast Plugin | ON     | Stream handling       |
| HomeKit            | ON     | HomeKit exposure      |
| Snapshot Plugin    | ON     | Thumbnail generation  |

### HomeKit Configuration

1. Enable **Standalone Accessory Mode** (required for notifications)
2. Pair in Home app with the unique code
3. Set notifications to **"Any Motion"**
4. Doorbell button will appear as a separate sensor

### Motion Zones (Optional)

If false positives are an issue with the wide-angle lens:
1. Use Scrypted's built-in motion detection zones
2. Or add the camera to Frigate for AI-based filtering (hybrid setup)

### Two-Way Audio

- Works natively through HomeKit
- Tap and hold the camera tile in Home app to talk
- Requires microphone permission on iOS

### Hybrid Setup (Advanced)

To combine Reolink doorbell features with Frigate object detection:

```
Reolink Doorbell → Reolink Plugin → HomeKit (doorbell/audio)
                 → Frigate → Frigate Bridge (motion with zones)
```

Configure carefully to avoid duplicate notifications:
1. Use Reolink Plugin for doorbell button and two-way audio
2. Use Frigate Bridge for motion detection (with zone filtering)
3. Disable motion detection on the Reolink Plugin side
