# Device Inventory

Per-device instance configurations. Each directory contains device-specific settings and references to the generic deployment configs in `edge/`.

## Devices

| Device | Tailscale IP | LAN IP | Role | Config Dir |
|--------|-------------|--------|------|------------|
| trv-pi-001 | <pi-001-tailscale-ip> | <pi-001-lan-ip> | ts-store + SenseHat | [trv-pi-001/](trv-pi-001/) |
| trv-pi-002 | <pi-002-tailscale-ip> | - | Mosquitto MQTT broker | [trv-pi-002/](trv-pi-002/) |
| trv-srv-001 | <hub-tailscale-ip> | - | Hub + ts-store stats | [trv-srv-001/](trv-srv-001/) |
| trv-srv-012 | <edge-srv-012-tailscale-ip> | - | EL operator (docker-compose) | [trv-srv-012/](trv-srv-012/) |
| trv-srv-014 | <edge-srv-014-tailscale-ip> | - | EL operator (Open Horizon) | [trv-srv-014/](trv-srv-014/) |
| jetson-nova | - | - | Motion detector | [jetson-nova/](jetson-nova/) |

## SSH Access

All devices are reachable via Tailscale. SSH user is `<user>` on all servers.

```bash
ssh <user>@<pi-001-tailscale-ip>     # trv-pi-001
ssh <user>@<pi-002-tailscale-ip>     # trv-pi-002
ssh <user>@<hub-tailscale-ip>     # trv-srv-001
ssh <user>@<edge-srv-012-tailscale-ip>   # trv-srv-012
ssh <user>@<edge-srv-014-tailscale-ip>     # trv-srv-014
```
