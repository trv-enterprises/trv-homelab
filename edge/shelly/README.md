# Shelly Power Monitor

Collector script for polling Shelly Gen2 devices (Plug, PM, etc.) and pushing power metrics to tsstore.

## Supported Devices

Any Shelly Gen2 device with power monitoring:
- Shelly Plug (Gen2/Gen4)
- Shelly PM
- Shelly Pro PM

## Metrics Collected

| Field | Description |
|-------|-------------|
| `output` | Switch state (true/false) |
| `apower` | Active power in watts |
| `voltage` | Voltage in V |
| `current` | Current in A |
| `freq` | Frequency in Hz |
| `temp_c` | Device temperature in Celsius |
| `energy_total_wh` | Cumulative energy in Wh |

## Configuration

The script reads configuration from environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `SHELLY_HOST` | `<cam-driveway-lan-ip>0` | Shelly device IP address |
| `SHELLY_SWITCH_ID` | `0` | Switch ID (usually 0) |
| `TSSTORE_SOCKET` | `/home/<user>/run/tsstore.sock` | tsstore Unix socket path |
| `TSSTORE_STORE` | `shelly-plug` | tsstore store name |
| `TSSTORE_API_KEY` | `CHANGEME` | tsstore API key for the store |
| `SAMPLE_INTERVAL` | `10` | Polling interval in seconds |

## Manual Testing

```bash
# Test Shelly endpoint directly
curl http://<cam-driveway-lan-ip>0/rpc/Switch.GetStatus?id=0

# Run collector manually
SHELLY_HOST=<cam-driveway-lan-ip>0 \
TSSTORE_STORE=shelly-plug-001 \
TSSTORE_API_KEY=your_api_key \
python3 shelly-to-tsstore.py
```

## Deployment

**Note:** Shelly plugs are now integrated via Zigbee2MQTT and Homebridge (see `edge/zigbee2mqtt/` and `edge/homebridge/`). This script is for legacy Wi-Fi-mode polling.
