# trv-srv-001 Device Config

Device-specific configs for trv-srv-001.

This directory covers the **ts-store and system-stats** services running on trv-srv-001.

## Services

| Service | Description |
|---------|-------------|
| tsstore | Native ts-store binary (port 21080) |
| system-stats | CPU/memory/disk/network collector |
| journal-logs | Journal log streamer |
| mqtt-sink-system-stats | Drains stats to MQTT broker |

## Files

- `tsstore.service` -- systemd unit for ts-store
- `stats-tsstore/` -- system-stats collector services and config

See [stats-tsstore/README.md](stats-tsstore/README.md) for setup details.
