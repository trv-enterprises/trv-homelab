# trv-pi-002

Mosquitto MQTT broker + tsstore with Shelly power monitoring.

- **Tailscale IP**: <pi-002-tailscale-ip>
- **Services**:
  - Mosquitto MQTT on port 1883
  - tsstore time-series database
  - Shelly power collector

## Directory Layout on Pi

```
~/bin/tsstore                              # binary
~/data/                                    # tsstore data directory
~/run/tsstore.sock                         # unix socket
~/.config/tsstore/                         # credentials (env files)
│   ├── admin.env                          # TSSTORE_ADMIN_KEY
│   └── shelly-plug-001.env                # store API key
~/tsstore/
├── scripts/
│   └── shelly-to-tsstore.py              # Shelly power collector (socket)
└── services/
    ├── tsstore.service
    └── shelly-to-tsstore.service
```

## Stores

| Store | Description | Protocol | Interval |
|-------|-------------|----------|----------|
| shelly-plug-001 | Shelly Plug power metrics | Unix socket | 10s |

## Deploy

```bash
# SSH to pi-002
ssh <user>@<pi-002-tailscale-ip>

# Clone/pull repo
cd ~/trv-homelab && git pull
# or: git clone https://github.com/trv-enterprises/trv-homelab.git

# Run deploy script
cd devices/trv-pi-002/scripts
./deploy.sh v0.3.0-rc1
```

### Post-Deploy Setup

After first deploy, generate credentials and create the store:

```bash
# Generate admin key
mkdir -p ~/.config/tsstore
echo "TSSTORE_ADMIN_KEY=tsstore_admin_$(cat /proc/sys/kernel/random/uuid)" > ~/.config/tsstore/admin.env

# Reload tsstore with admin key
systemctl --user daemon-reload
systemctl --user restart tsstore

# Create the store (save the API key from output!)
~/bin/tsstore create shelly-plug-001

# Save store API key
cat > ~/.config/tsstore/shelly-plug-001.env << EOF
TSSTORE_STORE=shelly-plug-001
TSSTORE_API_KEY=<paste_key_from_above>
EOF

# Start the collector
systemctl --user restart shelly-to-tsstore
```

## Services

All services run as user-level systemd units:

```bash
systemctl --user status tsstore
systemctl --user status shelly-to-tsstore

# Logs
journalctl --user -u shelly-to-tsstore -f
```

## Shelly Device

- **Device IP**: <cam-driveway-lan-ip>0
- **Type**: Shelly Plug Gen4 (or similar Gen2+ device)
- **Metrics**: apower, voltage, current, freq, temp_c, energy_total_wh

### Test Shelly Endpoint

```bash
curl http://<cam-driveway-lan-ip>0/rpc/Switch.GetStatus?id=0
```

## Mosquitto (existing)

The Mosquitto configuration is managed in [`edge/mosquitto/`](../../edge/mosquitto/).

```bash
# Check broker status
systemctl status mosquitto

# Subscribe to all topics
mosquitto_sub -h <pi-002-tailscale-ip> -t "#" -v
```
