# Caseta MQTT Bridge

Bidirectional bridge between a Lutron Caseta Smart Bridge (L-BDG2) and MQTT. Translates LEAP protocol events into MQTT messages and MQTT commands back into LEAP device control.

## Architecture

```
Caseta L-BDG2 (192.168.1.x)
    │ LEAP protocol (TLS, port 8081)
    ▼
caseta-bridge.service (Python, systemd)
    │ paho-mqtt / pylutron-caseta
    ▼
Mosquitto (localhost:1883)
    │ Topics: caseta/<device_name>, caseta/<device_name>/set
    ▼
Consumers (Homebridge, kiosk, alert engine, etc.)
```

Runs as a system-level systemd service (`caseta-bridge.service`) on the services LXC, alongside Mosquitto, Zigbee2MQTT, and Homebridge.

## MQTT Topics

### State (Caseta → MQTT)

Published on device state change, with `retain=true`.

**Topic**: `caseta/<device_name>`

**Payload**:
```json
{
  "device_id": "3",
  "name": "Media Room_Main Lights",
  "type": "light",
  "state": "on",
  "level": 100,
  "timestamp": "2026-03-17T00:32:50.167023+00:00"
}
```

| Field | Description |
|-------|-------------|
| `device_id` | Lutron internal device ID |
| `name` | Human-readable name from the Caseta bridge |
| `type` | `light`, `remote`, `shade`, `fan`, or `unknown` |
| `state` | `on` or `off` (derived from level) |
| `level` | 0–100 brightness/position |
| `timestamp` | ISO 8601 UTC timestamp |

### Button Events (Pico Remotes → MQTT)

Published when Pico remote buttons are pressed or released.

**Topic**: `caseta/<remote_name>/button`

**Payload**:
```json
{
  "device_id": "10",
  "name": "Living Room Pico",
  "button": 2,
  "action": "press",
  "timestamp": "2026-03-17T01:15:30.000000+00:00"
}
```

### Commands (MQTT → Caseta)

Subscribe and publish to control devices.

**Topic**: `caseta/<device_name>/set`

**Payloads**:
```json
{"action": "turn_on"}
{"action": "turn_off"}
{"action": "set_level", "level": 75}
{"action": "set_level", "level": 50, "fade_time": 2}
```

## Devices

Device names are auto-discovered from the Caseta bridge and sanitized to MQTT-safe topic segments (lowercase, underscores, alphanumeric only). Override names in `config.yaml`:

```yaml
device_names:
  "2": "kitchen_lights"
  "5": "living_room_dimmer"
```

## Setup

### 1. Pair with the Caseta Bridge

Requires physical access to press the button on the back of the L-BDG2:

```bash
make pair BRIDGE_IP=192.168.1.19
```

This generates TLS client certificates in `/etc/caseta-bridge/certs/`:
- `ca.crt` — Bridge CA certificate
- `client.crt` — Client certificate
- `client.key` — Client private key

**Important**: These certs cannot be regenerated without re-pairing. Back them up.

### 2. Configure

Edit `config.yaml` with the bridge IP, then deploy:

```yaml
caseta:
  bridge_host: "192.168.1.19"
  cert_dir: "/etc/caseta-bridge/certs"

mqtt:
  broker: "localhost"
  port: 1883
  client_id: "caseta-bridge"

topic_prefix: "caseta"
```

### 3. Deploy

```bash
make deploy              # Full deploy: sync files, install deps, start service
make deploy-config       # Config only (reloads via SIGHUP, no restart)
make deploy-code         # bridge.py only (restarts service)
```

## Operations

```bash
make status              # Show service status
make logs                # Follow journald logs
make restart             # Restart the service
make stop                # Stop the service
make devices             # Show discovered devices from logs
```

## Config Reload

The bridge supports live config reload via `SIGHUP` — refreshes device name mappings without dropping the LEAP or MQTT connections:

```bash
make deploy-config       # Or: sudo systemctl reload caseta-bridge
```

## Dependencies

- Python 3.10+
- `pylutron-caseta` — LEAP protocol client
- `paho-mqtt` — MQTT client
- `pyyaml` — Config parsing

Installed in a venv at `/opt/caseta-bridge/venv/`.
