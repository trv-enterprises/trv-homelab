# EdgeLake Kubernetes Operator (Go)

A Kubernetes Operator for deploying and managing EdgeLake operator nodes, built in Go using the controller-runtime library. This operator provides a native Kubernetes experience for managing EdgeLake deployments through Custom Resources.

## Overview

This operator watches for `EdgeLakeOperator` Custom Resources and automatically creates/manages:
- ConfigMaps with EdgeLake configuration
- PersistentVolumeClaims for data persistence
- Services for network access
- Deployments running the EdgeLake container

## Prerequisites

- Go 1.21+
- Docker
- Kubernetes cluster (v1.19+)
- kubectl configured to access your cluster

## Quick Start

### 1. Install CRDs

```bash
make install-crd
```

### 2. Build and Push Operator Image

```bash
# Set your registry
export IMG=your-registry/edgelake-operator:v1.0.0

# Build and push
make docker-build-push IMG=$IMG
```

### 3. Update Manager Image

Edit `config/manager/manager.yaml` and set the image to your pushed image:

```yaml
containers:
  - name: manager
    image: your-registry/edgelake-operator:v1.0.0
```

### 4. Deploy Operator

```bash
make deploy
```

### 5. Create an EdgeLakeOperator Instance

```bash
# Basic deployment
make deploy-sample-basic

# Or apply a custom CR
kubectl apply -f config/samples/edgelake_v1alpha1_basic.yaml
```

## Directory Structure

```
kube-operator-go/
├── api/
│   └── v1alpha1/
│       ├── groupversion_info.go       # API group registration
│       ├── edgelakeoperator_types.go  # CRD type definitions
│       └── zz_generated.deepcopy.go   # Generated deep copy methods
├── cmd/
│   └── manager/
│       └── main.go                    # Operator entrypoint
├── internal/
│   ├── controller/
│   │   └── edgelakeoperator_controller.go  # Reconciliation logic
│   └── resources/
│       ├── configmap.go               # ConfigMap builder
│       ├── deployment.go              # Deployment builder
│       ├── service.go                 # Service builder
│       ├── pvc.go                     # PVC builder
│       └── labels.go                  # Label helpers
├── config/
│   ├── crd/                           # CRD manifests
│   ├── rbac/                          # RBAC configuration
│   ├── manager/                       # Operator deployment
│   └── samples/                       # Example CRs
├── Dockerfile                         # Multi-stage build
├── Makefile                           # Build and deploy commands
├── go.mod                             # Go module definition
└── README.md                          # This file
```

## Custom Resource Definition

The `EdgeLakeOperator` CRD uses camelCase field names (Go convention) while the Helm chart uses snake_case:

```yaml
apiVersion: edgelake.trv.io/v1alpha1
kind: EdgeLakeOperator
metadata:
  name: my-edgelake-operator
  namespace: default
spec:
  metadata:
    hostname: edgelake-operator
    appName: edgelake-operator
    serviceType: NodePort

  image:
    repository: anylogco/edgelake-network
    tag: "1.3.2500"

  persistence:
    enabled: true
    data:
      size: 10Gi

  resources:
    limits:
      cpu: "2000m"
      memory: "4Gi"

  nodeConfigs:
    general:
      nodeType: operator
      companyName: "My Company"
    networking:
      anylogServerPort: 32148
      anylogRESTPort: 32149
    blockchain:
      ledgerConn: "master-ip:32048"
    operator:
      clusterName: my-cluster
      defaultDBMS: my_data
```

## Configuration Reference

### metadata

| Field | Description | Default |
|-------|-------------|---------|
| `hostname` | Deployment hostname | `edgelake-operator` |
| `appName` | Application name | CR name |
| `serviceName` | Service name | `<name>-service` |
| `configMapName` | ConfigMap name | `<name>-configmap` |
| `nodeSelector` | Pod scheduling constraints | `{}` |
| `serviceType` | Service type | `NodePort` |

### image

| Field | Description | Default |
|-------|-------------|---------|
| `repository` | Docker image repository | `anylogco/edgelake-network` |
| `tag` | Image tag | `1.3.2500` |
| `pullPolicy` | Image pull policy | `IfNotPresent` |
| `secretName` | Image pull secret | `""` |

### persistence

| Field | Description | Default |
|-------|-------------|---------|
| `enabled` | Enable persistent volumes | `true` |
| `storageClassName` | Storage class | `""` (default) |
| `accessMode` | PVC access mode | `ReadWriteOnce` |
| `anylog.size` | AnyLog volume size | `5Gi` |
| `blockchain.size` | Blockchain volume size | `1Gi` |
| `data.size` | Data volume size | `10Gi` |
| `scripts.size` | Scripts volume size | `1Gi` |

### nodeConfigs

Complete configuration options for EdgeLake nodes. Key sections include:

- **general**: Node type, name, company
- **networking**: Ports, overlay IP, thread configuration
- **database**: SQLite or PostgreSQL configuration
- **blockchain**: Master node connection, sync settings
- **operator**: Cluster name, partitioning, HA settings
- **mqtt**: MQTT client configuration
- **opcua**: OPC-UA client settings
- **monitoring**: Node monitoring options
- **advanced**: Performance tuning

## Development

### Run Locally

```bash
# Install CRDs first
make install-crd

# Run the operator locally (outside cluster)
make run
```

### Build Binary

```bash
make build
# Binary output: bin/manager
```

### Run Tests

```bash
make test
```

### Generate Code

After modifying types in `api/v1alpha1/`:

```bash
make generate
```

## Deployment

### Full Installation

```bash
make install
```

This runs:
1. `install-crd` - Installs the CRD
2. `deploy` - Deploys the operator

### View Status

```bash
make status
```

### View Logs

```bash
make logs
```

### Uninstall

```bash
make uninstall
```

## Sample Deployments

### Basic (SQLite)

```bash
kubectl apply -f config/samples/edgelake_v1alpha1_basic.yaml
```

### Production (PostgreSQL)

```bash
kubectl create namespace edgelake
kubectl apply -f config/samples/edgelake_v1alpha1_production.yaml
```

### MQTT Data Ingestion

```bash
kubectl create namespace iot-data
kubectl apply -f config/samples/edgelake_v1alpha1_mqtt.yaml
```

## Status and Conditions

The operator updates the CR status with:

- **phase**: `Pending`, `Creating`, `Running`, or `Failed`
- **ready**: Boolean indicating if deployment is ready
- **conditions**: Detailed status conditions
- **deploymentName**: Name of the created deployment
- **serviceName**: Name of the created service
- **configMapName**: Name of the created ConfigMap

View status:

```bash
kubectl get edgelakeoperators
kubectl describe edgelakeoperator <name>
```

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                  Kubernetes Cluster                  │
├─────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────┐   │
│  │         EdgeLake Operator (Go)               │   │
│  │  ┌─────────────┐   ┌──────────────────────┐ │   │
│  │  │ Controller  │──▶│  Reconcile Loop      │ │   │
│  │  └─────────────┘   └──────────┬───────────┘ │   │
│  └───────────────────────────────┼─────────────┘   │
│                                  │                  │
│                    ┌─────────────▼─────────────┐   │
│                    │     EdgeLakeOperator CR    │   │
│                    │  apiVersion: edgelake.trv.io/v1alpha1 │
│                    └─────────────┬─────────────┘   │
│                                  │                  │
│         ┌────────────────────────┼────────────────┐│
│         │        Managed Resources                ││
│         │  ┌───────────┐  ┌───────────┐          ││
│         │  │ ConfigMap │  │  Service  │          ││
│         │  └───────────┘  └───────────┘          ││
│         │  ┌───────────┐  ┌───────────┐          ││
│         │  │Deployment │  │   PVCs    │          ││
│         │  └─────┬─────┘  └───────────┘          ││
│         │        │                                ││
│         │  ┌─────▼─────┐                         ││
│         │  │ EdgeLake  │                         ││
│         │  │   Pod     │                         ││
│         │  └───────────┘                         ││
│         └─────────────────────────────────────────┘│
└─────────────────────────────────────────────────────┘
```

## Comparison with Helm-based Operator

| Feature | Go Operator | Helm Operator |
|---------|-------------|---------------|
| Language | Go | YAML/Helm |
| Customization | Full control | Limited to Helm |
| Status Handling | Native conditions | Basic |
| Validation | Go types + CRD | CRD only |
| Testing | Go tests | Helm lint |
| Build | Compile + Docker | Docker only |
| Runtime | controller-runtime | helm-operator |

## Related Documentation

- [EdgeLake Documentation](https://github.com/EdgeLake/EdgeLake)
- [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime)
- [Kubebuilder Book](https://book.kubebuilder.io/)
