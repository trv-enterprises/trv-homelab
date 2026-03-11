# trv-srv-012

EdgeLake operator node deployed via docker-compose.

- **Tailscale IP**: <edge-srv-012-tailscale-ip>
- **EdgeLake ports**: 32148 (TCP), 32149 (REST)
- **Node name**: edgelake-operator

## Deployment

This device uses the docker-compose deployment method. Configuration is at:

- [`edge/edgelake-operator/docker-compose/deployments/operator/`](../../edge/edgelake-operator/docker-compose/deployments/operator/)
- [`edge/edgelake-operator/docker-compose/deployments/operator2/`](../../edge/edgelake-operator/docker-compose/deployments/operator2/)

```bash
cd edge/edgelake-operator/docker-compose
make sync operator        # Sync to trv-srv-012
make up operator          # Start operator
make status operator      # Check status
```
