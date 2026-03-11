# trv-srv-014

EdgeLake operator node deployed via Open Horizon.

- **Tailscale IP**: <edge-srv-014-tailscale-ip>
- **EdgeLake ports**: 32248 (TCP), 32249 (REST), 32150 (Broker)
- **Node name**: edgelake-operator-oh

## Deployment

This device uses the Open Horizon deployment method. Configuration is at:

- [`edge/edgelake-operator/open-horizon/configurations/operator_production.env`](../../edge/edgelake-operator/open-horizon/configurations/operator_production.env)

```bash
# Publish from hub
cd hub/open-horizon
make -f Makefile.oh oh-publish-all

# On the device, register with OH
hzn register --policy=node.policy.json
```

## Notes

- OH containers do NOT support TTY -- use helper script `~/edgelake-cli.sh` for CLI access
- See [`edge/edgelake-operator/open-horizon/`](../../edge/edgelake-operator/open-horizon/) for full documentation
