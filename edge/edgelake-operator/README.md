# EdgeLake Operator Deployment

Multiple methods for deploying EdgeLake operator nodes to edge devices.

## Deployment Methods

| Method | Directory | Best For |
|--------|-----------|----------|
| Docker Compose | [docker-compose/](docker-compose/) | Direct deployment, simple setup |
| Open Horizon | [open-horizon/](open-horizon/) | Managed fleet deployment |
| Helm Charts | [helm-charts/](helm-charts/) | Kubernetes clusters |
| Kubernetes Operator | [kube-operator-go/](kube-operator-go/) | Native K8s operator pattern |

## Docker Compose

Direct docker-compose deployment. Currently used on trv-srv-012.

```bash
cd docker-compose
make up operator          # Start operator
make status operator      # Check status
make attach operator      # Attach to CLI
```

Deployments: `operator`, `operator2`

## Open Horizon

Open Horizon-managed deployment via the OH Exchange. Currently used on trv-srv-014.

Service definitions, node policies, and per-target configuration profiles are in `open-horizon/`.

## Helm Charts

Kubernetes Helm chart for deploying EdgeLake operator in K8s clusters. Includes cloud deployment guides (AWS, GCP) and testing scripts.

```bash
cd helm-charts
helm install edgelake-operator edgelake-operator/ -f edgelake-operator/examples/production-psql.yaml
```

## Kubernetes Operator (Go)

Full Go-based Kubernetes operator with CRDs, RBAC, and Open Horizon integration.

```bash
cd kube-operator-go
make docker-build
make deploy
```
