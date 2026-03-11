# Open Horizon Deployment for EdgeLake Kubernetes Operator

This directory contains the Open Horizon service definition and deployment artifacts for deploying the EdgeLake Kubernetes Operator to edge clusters via Open Horizon.

## Overview

Open Horizon can deploy Kubernetes operators to edge clusters using a `clusterDeployment` service type. The operator YAML files are packaged into a tar.gz archive and published to the OH Exchange. When a Kubernetes edge cluster matches the deployment policy, OH deploys the operator which then manages EdgeLake instances.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Open Horizon Hub                          │
│  ┌─────────────────┐  ┌─────────────────────────────────┐  │
│  │    Exchange     │  │  EdgeLake Kube-Operator Service │  │
│  │   (Registry)    │  │  - operator-yamls.tar.gz        │  │
│  └────────┬────────┘  │  - service.definition.json      │  │
│           │           │  - deployment.policy.json       │  │
│           │           └─────────────────────────────────┘  │
└───────────┼─────────────────────────────────────────────────┘
            │
            │ Agreement
            ▼
┌─────────────────────────────────────────────────────────────┐
│              Kubernetes Edge Cluster                         │
│  ┌─────────────────────────────────────────────────────────┐│
│  │                OH Agent (openhorizon-agent ns)           ││
│  │  - Receives agreement                                    ││
│  │  - Extracts operator-yamls.tar.gz                       ││
│  │  - Applies YAML files via kubectl                       ││
│  └──────────────────────────┬──────────────────────────────┘│
│                             │                                │
│  ┌──────────────────────────▼──────────────────────────────┐│
│  │         EdgeLake Operator (edgelake-operator-system)     ││
│  │  - CRD: EdgeLakeOperator                                ││
│  │  - Deployment: controller-manager                       ││
│  │  - Watches for EdgeLakeOperator CRs                     ││
│  └──────────────────────────┬──────────────────────────────┘│
│                             │ Creates                        │
│  ┌──────────────────────────▼──────────────────────────────┐│
│  │              EdgeLake Instance (default ns)              ││
│  │  - ConfigMap: edgelake configuration                    ││
│  │  - PVCs: anylog, blockchain, data, scripts              ││
│  │  - Deployment: EdgeLake container                       ││
│  │  - Service: TCP/REST/Broker ports                       ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

1. **Open Horizon Management Hub** configured and running
2. **Kubernetes Edge Cluster** with OH agent installed
3. **Operator Container Image** built and pushed to registry
4. **hzn CLI** installed and configured

## Directory Structure

```
oh-integration/
├── Makefile                    # Build and publish commands
├── README.md                   # This file
├── service.definition.json     # OH service definition (cluster type)
├── service.policy.json         # Service policy
├── deployment.policy.json      # Deployment policy
├── node.policy.json            # Node policy for edge clusters
├── configurations/             # Configuration templates
└── operator-yamls/             # Kubernetes YAML files
    ├── 00-namespace.yaml       # Operator namespace
    ├── 01-crd.yaml             # EdgeLakeOperator CRD
    ├── 02-service-account.yaml # Service account
    ├── 03-cluster-role.yaml    # RBAC role
    ├── 04-cluster-role-binding.yaml
    ├── 05-operator-deployment.yaml  # Operator deployment
    └── 06-edgelake-cr.yaml     # EdgeLake CR template
```

## Quick Start

### 1. Build and Push Operator Image

First, build the Go operator and push to your registry:

```bash
cd /path/to/kube-operator-go

# Build the operator image
docker build -t <hub-tailscale-ip>:5000/edgelake-operator:1.0.0 .

# Push to registry
docker push <hub-tailscale-ip>:5000/edgelake-operator:1.0.0
```

### 2. Configure Environment

```bash
cd oh-integration

# Set OH credentials
export HZN_ORG_ID=myorg
export HZN_EXCHANGE_USER_AUTH=myorg/admin:your-password
export HZN_EXCHANGE_URL=http://<hub-tailscale-ip>:3090/v1

# Set service version and operator image
export SERVICE_VERSION=1.0.0
export OPERATOR_IMAGE=<hub-tailscale-ip>:5000/edgelake-operator:1.0.0
```

### 3. Build and Publish

```bash
# Build the tar.gz archive and publish to Exchange
make publish-all
```

### 4. Register Edge Cluster

On the Kubernetes edge cluster:

```bash
# Ensure OH agent is running
kubectl get pods -n openhorizon-agent

# Register with node policy
export HZN_ORG_ID=myorg
export HZN_EXCHANGE_USER_AUTH=myorg/admin:your-password
hzn register --policy=node.policy.json

# Monitor agreement formation
watch hzn agreement list
```

### 5. Verify Deployment

```bash
# Check operator is running
kubectl get pods -n edgelake-operator-system

# Check EdgeLake CR was created
kubectl get edgelakeoperators

# Check EdgeLake instance
kubectl get pods -l app=edgelake-operator
kubectl get svc | grep edgelake
```

## Service Definition

The service uses `clusterDeployment` type for Kubernetes:

```json
{
  "clusterDeployment": {
    "operatorYamlArchive": "operator-yamls.tar.gz"
  }
}
```

Key features:
- YAML files are extracted and applied in alphabetical order
- Variables in YAML files are substituted from `userInput`
- Operator manages the full lifecycle of EdgeLake instances

## Configuration Architecture

Configuration is split between **network-wide defaults** (in service definition) and **per-cluster settings** (in node policy):

```
┌─────────────────────────────────────────────────────────────────┐
│                     Service Definition                           │
│  Network-wide defaults (same for all clusters):                 │
│  - COMPANY_NAME, LEDGER_CONN, DB_TYPE, IMAGE_TAG, etc.         │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ defaults
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Deployment Policy                            │
│  Generic targeting only:                                        │
│  - constraints: purpose == edgelake-kubernetes                  │
│  - No userInput (relies on service defaults + node overrides)   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ matches
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Node Policy                                │
│  Per-cluster configuration:                                     │
│  - NODE_NAME, OVERLAY_IP, CLUSTER_NAME, SERVER_PORT, REST_PORT │
└─────────────────────────────────────────────────────────────────┘
```

### Network-wide Parameters (Service Definition Defaults)

| Parameter | Description | Default |
|-----------|-------------|---------|
| `EDGELAKE_COMPANY_NAME` | Organization name | New Company |
| `EDGELAKE_LEDGER_CONN` | Master node connection | <hub-tailscale-ip>:32048 |
| `EDGELAKE_DB_TYPE` | Database type | sqlite |
| `EDGELAKE_DEFAULT_DBMS` | Default database name | production_data |
| `EDGELAKE_IMAGE_TAG` | EdgeLake container tag | 1.3.2500 |
| `EDGELAKE_BROKER_PORT` | MQTT broker port | 0 (disabled) |
| `EDGELAKE_STORAGE_CLASS` | K8s storage class | "" (default) |
| `EDGELAKE_SERVICE_TYPE` | K8s service type | NodePort |
| `EDGELAKE_ENABLE_MQTT` | Enable MQTT ingestion | false |
| `EDGELAKE_MQTT_BROKER` | MQTT broker address | "" |

### Per-Cluster Parameters (Node Policy)

| Parameter | Description | Example |
|-----------|-------------|---------|
| `EDGELAKE_NODE_NAME` | Unique name for this cluster's EdgeLake instance | edgelake-operator-cluster-a |
| `EDGELAKE_OVERLAY_IP` | VPN/Tailscale IP for this cluster | 100.x.x.x |
| `EDGELAKE_CLUSTER_NAME` | Operator cluster name | cluster-a |
| `EDGELAKE_SERVER_PORT` | TCP server port | 32148 |
| `EDGELAKE_REST_PORT` | REST API port | 32149 |

## Makefile Commands

```bash
# Build
make build-tar          # Create operator-yamls.tar.gz
make build              # Build all artifacts

# Publish
make publish-service    # Publish service to Exchange
make publish-deployment # Publish deployment policy
make publish-all        # Publish everything

# Inspect
make list-services      # List published services
make show-service       # Show service details
make show-deployment    # Show deployment policy

# Cleanup
make remove-service     # Remove service from Exchange
make remove-deployment  # Remove deployment policy
make clean              # Remove local generated files

# Validation
make check-vars         # Validate configuration
make test-connection    # Test Exchange connection
```

## Customizing Deployment

### Registering a New Cluster

Each Kubernetes cluster gets its own node policy with cluster-specific configuration. Create a node policy file for each cluster:

```bash
# Create node policy for cluster-a
cat > node-policy-cluster-a.json << EOF
{
  "properties": [
    {"name": "purpose", "value": "edgelake-kubernetes"},
    {"name": "openhorizon.kubernetesEnabled", "value": true},
    {"name": "cluster_type", "value": "kubernetes"}
  ],
  "constraints": [],
  "deployment": {
    "services": [
      {
        "org": "myorg",
        "url": "edgelake-kube-operator",
        "versionRange": "[0.0.0,INFINITY)",
        "variables": {
          "EDGELAKE_NODE_NAME": "edgelake-operator-cluster-a",
          "EDGELAKE_OVERLAY_IP": "100.74.102.38",
          "EDGELAKE_CLUSTER_NAME": "cluster-a",
          "EDGELAKE_SERVER_PORT": 32148,
          "EDGELAKE_REST_PORT": 32149
        }
      }
    ]
  }
}
EOF

# Register the cluster
hzn register --policy=node-policy-cluster-a.json
```

### Adding Another Cluster

```bash
# Create node policy for cluster-b (different IP, ports, name)
cat > node-policy-cluster-b.json << EOF
{
  "properties": [
    {"name": "purpose", "value": "edgelake-kubernetes"},
    {"name": "openhorizon.kubernetesEnabled", "value": true},
    {"name": "cluster_type", "value": "kubernetes"}
  ],
  "constraints": [],
  "deployment": {
    "services": [
      {
        "org": "myorg",
        "url": "edgelake-kube-operator",
        "versionRange": "[0.0.0,INFINITY)",
        "variables": {
          "EDGELAKE_NODE_NAME": "edgelake-operator-cluster-b",
          "EDGELAKE_OVERLAY_IP": "100.91.45.12",
          "EDGELAKE_CLUSTER_NAME": "cluster-b",
          "EDGELAKE_SERVER_PORT": 32148,
          "EDGELAKE_REST_PORT": 32149
        }
      }
    ]
  }
}
EOF

# Register cluster-b
hzn register --policy=node-policy-cluster-b.json
```

### Overriding Network-wide Defaults

If a specific cluster needs different network-wide settings (e.g., different master node), add those to the node policy variables:

```bash
# Cluster pointing to a different master
"variables": {
  "EDGELAKE_NODE_NAME": "edgelake-operator-site-b",
  "EDGELAKE_OVERLAY_IP": "100.50.60.70",
  "EDGELAKE_CLUSTER_NAME": "site-b",
  "EDGELAKE_SERVER_PORT": 32148,
  "EDGELAKE_REST_PORT": 32149,
  "EDGELAKE_LEDGER_CONN": "100.50.60.1:32048"
}
```

## Troubleshooting

### Agreement Not Forming

```bash
# Check node is registered
hzn node list

# Check node policy
hzn policy list

# Check for errors
hzn eventlog list | tail -50

# Verify service exists
hzn exchange service list myorg/ | grep edgelake
```

### Operator Not Deploying

```bash
# Check OH agent logs
kubectl logs -n openhorizon-agent -l app=agent

# Check for YAML extraction issues
kubectl get events -n openhorizon-agent

# Check operator namespace
kubectl get all -n edgelake-operator-system
```

### EdgeLake Instance Issues

```bash
# Check operator logs
kubectl logs -n edgelake-operator-system -l control-plane=controller-manager

# Check EdgeLake CR status
kubectl describe edgelakeoperator <name>

# Check EdgeLake pod
kubectl logs -l app=edgelake-operator
```

## Network Considerations

### Tailscale/Overlay Network

When using Tailscale on the K8s host (not in cluster):

1. Set `EDGELAKE_OVERLAY_IP` to the host's Tailscale IP in the node policy
2. Use `NodePort` service type (default)
3. Ensure K8s NodePort range includes EdgeLake ports

```bash
# Check Tailscale IP on host
tailscale ip -4

# Set in node policy variables
"variables": {
  "EDGELAKE_OVERLAY_IP": "100.x.x.x",
  ...
}
```

### Port Conflicts

Each EdgeLake instance needs unique ports. Standard convention:

| Node Type | TCP Port | REST Port | Broker Port |
|-----------|----------|-----------|-------------|
| Master    | 32048    | 32049     | -           |
| Operator 1| 32148    | 32149     | 32150       |
| Operator 2| 32248    | 32249     | 32250       |
| Query     | 32348    | 32349     | -           |

## References

- [Open Horizon Kubernetes Operators](https://open-horizon.github.io/docs/developing/service_operators/)
- [Deploying to Edge Clusters](https://open-horizon.github.io/docs/using_edge_services/deploying_services_cluster/)
- [EdgeLake Documentation](https://github.com/EdgeLake/EdgeLake)
- [OH Hub Learnings](../../../OH_HUB_LEARNINGS.md)

Sources:
- [Developing a Kubernetes operator | Open Horizon](https://open-horizon.github.io/docs/developing/service_operators/)
- [Deploying services to an edge cluster | Open Horizon](https://open-horizon.github.io/docs/using_edge_services/deploying_services_cluster/)
- [Edge cluster service | Open Horizon](https://open-horizon.github.io/docs/using_edge_services/edge_cluster_service/)
