# ts-store and system-stats deployment for trv-srv-001

Native ts-store installation with system-stats collector on trv-srv-001.

## Architecture

- **ts-store**: Native systemd service (not Docker)
  - HTTP API on port 21080
  - Unix socket at `/var/run/tsstore/tsstore.sock`
  - Data stored at `/home/<user>/data/tsstore`

- **system-stats**: Collector using Unix socket for low-latency writes
  - Collects CPU, memory, disk I/O, network, and disk space
  - Writes to `system-stats` store every 20 seconds
  - Uses schema-type store for compact storage (~50% smaller)

- **mqtt-sink**: Drains system-stats to MQTT broker
  - Reads from ts-store HTTP API
  - Publishes to Mosquitto on trv-pi-002 (<pi-002-tailscale-ip>)
  - Maintains cursor for crash recovery
  - Topic: `trv-srv-001/system-stats`

## Installation

### 1. Build binaries

```bash
cd /path/to/ts-store

# Build ts-store
GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o tsstore-linux-amd64 ./cmd/tsstore

# Build system-stats
cd examples/system-stats
GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o system-stats-linux-amd64 .
```

### 2. Deploy binaries

```bash
scp tsstore-linux-amd64 <user>@<hub-tailscale-ip>:~/bin/tsstore
scp system-stats-linux-amd64 <user>@<hub-tailscale-ip>:~/bin/system-stats
ssh <user>@<hub-tailscale-ip> "chmod +x ~/bin/tsstore ~/bin/system-stats"
```

### 3. Create directories

```bash
ssh <user>@<hub-tailscale-ip> "mkdir -p ~/data/tsstore"
sudo mkdir -p /var/run/tsstore
sudo chown <user>:<user> /var/run/tsstore
```

### 4. Install service files

```bash
sudo cp tsstore.service /etc/systemd/system/
sudo cp system-stats.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable tsstore system-stats
```

### 5. Start ts-store and create the store

```bash
sudo systemctl start tsstore

# Create system-stats store (schema type for compact storage)
curl -X POST http://localhost:21080/api/stores \
  -H "X-Admin-Key: CHANGEME" \
  -H "Content-Type: application/json" \
  -d '{"name": "system-stats", "num_blocks": 10000, "data_type": "schema"}'

# Save the returned API key to .env and update system-stats.service

# Set the schema
curl -X PUT http://localhost:21080/api/stores/system-stats/schema \
  -H "X-API-Key: <api-key>" \
  -H "Content-Type: application/json" \
  -d '{"fields":[{"index":1,"name":"cpu.pct","type":"int32"},{"index":2,"name":"memory.total","type":"int64"},{"index":3,"name":"memory.used","type":"int64"},{"index":4,"name":"memory.available","type":"int64"},{"index":5,"name":"memory.pct","type":"int32"},{"index":6,"name":"disk_io.read_bytes_sec","type":"int64"},{"index":7,"name":"disk_io.write_bytes_sec","type":"int64"},{"index":8,"name":"network.rx_bytes_sec","type":"int64"},{"index":9,"name":"network.tx_bytes_sec","type":"int64"},{"index":10,"name":"disk_space.total","type":"int64"},{"index":11,"name":"disk_space.used","type":"int64"},{"index":12,"name":"disk_space.available","type":"int64"},{"index":13,"name":"disk_space.pct","type":"int32"}]}'
```

### 6. Start system-stats

```bash
sudo systemctl start system-stats
```

## Verification

```bash
# Check services are running
systemctl status tsstore system-stats

# Check data is flowing
curl -s -H "X-API-Key: <api-key>" \
  "http://localhost:21080/api/stores/system-stats/data/newest?limit=1" | jq .
```

## Files

- `.env` - API keys (not in git, contains actual keys)
- `.env.example` - Template for .env
- `system-stats.service` - Systemd service for stats collector
- `journal-logs.service` - Systemd service for journal collector
- `mqtt-sink-system-stats.service` - Systemd service for MQTT sink
- `../tsstore.service` - Systemd service for ts-store

## Current Configuration

### ts-store
- **Port**: 21080
- **Socket**: /var/run/tsstore/tsstore.sock
- **Data path**: /home/<user>/data/tsstore

### system-stats store
- **Type**: schema (compact)
- **Blocks**: 10,000
- **Collection interval**: 20 seconds
- **Retention**: ~28 days

### journal-logs store
- **Type**: json
- **Blocks**: 50,000
- **Collection**: real-time streaming
- **Retention**: depends on log volume

### mqtt-sink
- **Broker**: tcp://<pi-002-tailscale-ip>:1883 (trv-pi-002)
- **Topic**: trv-srv-001/system-stats
- **Cursor file**: /var/lib/mqtt-sink/system-stats.cursor

## MQTT Sink Setup

```bash
# Create cursor directory
sudo mkdir -p /var/lib/mqtt-sink
sudo chown <user>:<user> /var/lib/mqtt-sink

# Deploy mqtt-sink binary
scp mqtt-sink-linux-amd64 <user>@<hub-tailscale-ip>:~/bin/mqtt-sink
ssh <user>@<hub-tailscale-ip> "chmod +x ~/bin/mqtt-sink"

# Install and start service
sudo cp mqtt-sink-system-stats.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable mqtt-sink-system-stats
sudo systemctl start mqtt-sink-system-stats
```
