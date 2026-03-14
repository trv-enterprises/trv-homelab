# Device Inventory

Per-device instance configurations. Each directory contains device-specific settings and references to the generic deployment configs in `edge/`.

## Devices

| Device | Tailscale IP | LAN IP | Role | Config Dir |
|--------|-------------|--------|------|------------|
| trv-pi-001 | <pi-001-tailscale-ip> | <pi-001-lan-ip> | ts-store + SenseHat | [trv-pi-001/](trv-pi-001/) |
| trv-srv-001 | <hub-tailscale-ip> | - | ts-store stats | [trv-srv-001/](trv-srv-001/) |
| jetson-nova | - | - | Motion detector | [jetson-nova/](jetson-nova/) |

## SSH Access

All devices are reachable via Tailscale. SSH user is `<user>` on all servers.

```bash
ssh <user>@<pi-001-tailscale-ip>     # trv-pi-001
ssh <user>@<hub-tailscale-ip>        # trv-srv-001
```
