# Mosquitto MQTT Broker

Generic Mosquitto MQTT broker deployment for receiving data from ts-store sinks and IoT devices.

## Installation

```bash
# Install Mosquitto
sudo apt update
sudo apt install -y mosquitto mosquitto-clients

# Stop service to configure
sudo systemctl stop mosquitto

# Backup default config
sudo mv /etc/mosquitto/mosquitto.conf /etc/mosquitto/mosquitto.conf.bak

# Copy our config
sudo cp mosquitto.conf /etc/mosquitto/mosquitto.conf

# Create log directory
sudo mkdir -p /var/log/mosquitto
sudo chown mosquitto:mosquitto /var/log/mosquitto

# Start and enable
sudo systemctl start mosquitto
sudo systemctl enable mosquitto
```

## Verification

```bash
# Check service status
systemctl status mosquitto

# Test with subscriber (in one terminal)
mosquitto_sub -h localhost -t "test/#" -v

# Test with publisher (in another terminal)
mosquitto_pub -h localhost -t "test/hello" -m "world"

# Subscribe to ts-store data
mosquitto_sub -h localhost -t "trv-srv-001/#" -v
```

## Configuration

| Setting | Value | Description |
|---------|-------|-------------|
| Port | 1883 | Standard MQTT port |
| Anonymous | true | No authentication (internal network) |
| Persistence | true | Retain messages across restarts |
| Log location | /var/log/mosquitto/mosquitto.log | Log file path |

## Expected Topics

| Topic | Source | Description |
|-------|--------|-------------|
| trv-srv-001/system-stats | mqtt-sink | System stats from trv-srv-001 |

## Security Notes

This configuration allows anonymous connections. For production use on untrusted networks:

1. Set `allow_anonymous false`
2. Add password file:
   ```
   password_file /etc/mosquitto/passwd
   ```
3. Create passwords:
   ```bash
   sudo mosquitto_passwd -c /etc/mosquitto/passwd username
   ```

## CLI Commands

### Subscribing

```bash
# Subscribe to a specific topic
mosquitto_sub -h localhost -t "trv-srv-001/system-stats" -v

# Subscribe to all topics under a prefix
mosquitto_sub -h localhost -t "trv-srv-001/#" -v

# Subscribe to ALL topics (useful for debugging)
mosquitto_sub -h localhost -t "#" -v

# Get just one message and exit
mosquitto_sub -h localhost -t "trv-srv-001/system-stats" -C 1

# Get 10 messages and exit
mosquitto_sub -h localhost -t "trv-srv-001/system-stats" -C 10

# Subscribe from a remote machine
mosquitto_sub -h <broker-ip> -t "trv-srv-001/system-stats" -v
```

### Publishing

```bash
# Publish a simple message
mosquitto_pub -h localhost -t "test/hello" -m "world"

# Publish JSON
mosquitto_pub -h localhost -t "test/data" -m '{"temp": 22.5}'

# Publish from stdin
echo '{"value": 42}' | mosquitto_pub -h localhost -t "test/data" -l
```

### Common Options

| Option | Description |
|--------|-------------|
| `-h` | Hostname (default: localhost) |
| `-p` | Port (default: 1883) |
| `-t` | Topic |
| `-v` | Verbose - print topic name with message |
| `-C <n>` | Exit after receiving n messages |
| `-m` | Message payload |
| `-l` | Read messages from stdin, one per line |
| `-q` | QoS level (0, 1, or 2) |
| `-u` | Username |
| `-P` | Password |

### Topic Wildcards

- `+` matches one level: `sensors/+/temp` matches `sensors/room1/temp`
- `#` matches all remaining levels: `sensors/#` matches `sensors/room1/temp/celsius`

## Logs

```bash
# View logs
sudo tail -f /var/log/mosquitto/mosquitto.log

# Check for errors
sudo journalctl -u mosquitto -f
```
