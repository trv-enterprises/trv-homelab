# EdgeLake Operator Helm Chart Installation Guide

## Quick Installation Steps

### 1. Prerequisites

Ensure you have:
- Kubernetes cluster (1.19+)
- Helm 3.0+ installed
- kubectl configured to access your cluster
- A running EdgeLake Master node (for blockchain/metadata)

### 2. Basic Installation

```bash
# Install with default values
helm install my-operator ./edgelake-operator
```

### 3. Verify Installation

```bash
# Check pod status
kubectl get pods -l app=edgelake-operator

# View logs
kubectl logs -l app=edgelake-operator -f

# Check service
kubectl get svc edgelake-operator-service
```

## Production Installation

### Step 1: Create Custom Values File

Create `production-values.yaml`:

```yaml
metadata:
  namespace: edgelake-prod
  app_name: operator-node-1
  service_type: NodePort

image:
  repository: anylogco/edgelake-network
  tag: "1.3.2500"
  pull_policy: IfNotPresent

persistence:
  enabled: true
  storageClassName: "fast-ssd"
  data:
    size: 100Gi
  anylog:
    size: 10Gi
  blockchain:
    size: 2Gi

resources:
  limits:
    cpu: "4000m"
    memory: "8Gi"
  requests:
    cpu: "1000m"
    memory: "2Gi"

node_configs:
  general:
    NODE_NAME: "operator-node-1"
    COMPANY_NAME: "Your Company"

  networking:
    ANYLOG_SERVER_PORT: 32148
    ANYLOG_REST_PORT: 32149
    OVERLAY_IP: ""  # Optional: Set if using VPN

  blockchain:
    # IMPORTANT: Set to your master node IP and port
    LEDGER_CONN: "10.0.0.10:32048"
    BLOCKCHAIN_SOURCE: master

  database:
    # Use PostgreSQL for production
    DB_TYPE: psql
    DB_USER: edgelake
    DB_PASSWD: "your-secure-password"
    DB_IP: postgres-service.default.svc.cluster.local
    DB_PORT: 5432

  operator:
    CLUSTER_NAME: production-cluster
    DEFAULT_DBMS: production_data
    ENABLE_PARTITIONS: true
    PARTITION_INTERVAL: "7 days"
    PARTITION_KEEP: 4
    OPERATOR_THREADS: 6

  advanced:
    QUERY_POOL: 12
    THRESHOLD_TIME: "30 seconds"
    THRESHOLD_VOLUME: "1MB"
```

### Step 2: Create Namespace

```bash
kubectl create namespace edgelake-prod
```

### Step 3: Install with Custom Values

```bash
helm install operator-node-1 ./edgelake-operator \
  -f production-values.yaml \
  --namespace edgelake-prod
```

### Step 4: Verify Installation

```bash
# Check all resources
kubectl get all -n edgelake-prod -l app=edgelake-operator

# Check persistent volumes
kubectl get pvc -n edgelake-prod

# View logs
kubectl logs -n edgelake-prod -l app=edgelake-operator -f

# Get node IP for NodePort access
kubectl get nodes -o wide

# Get service details
kubectl get svc -n edgelake-prod edgelake-operator-service
```

## Common Installation Scenarios

### Scenario 1: Development (SQLite, No Persistence)

```yaml
metadata:
  service_type: NodePort

node_configs:
  blockchain:
    LEDGER_CONN: "127.0.0.1:32048"

  database:
    DB_TYPE: sqlite

persistence:
  enabled: false

resources:
  limits:
    cpu: "1000m"
    memory: "2Gi"
```

Install:
```bash
helm install dev-operator ./edgelake-operator -f dev-values.yaml
```

### Scenario 2: MQTT Data Ingestion

```yaml
node_configs:
  mqtt:
    ENABLE_MQTT: true
    MQTT_BROKER: "mqtt-broker.iot.svc.cluster.local"
    MQTT_PORT: 1883
    MQTT_USER: "edgelake"
    MQTT_PASSWD: "mqtt-password"
    MSG_TOPIC: "sensors/+/data"
    MSG_DBMS: iot_data
```

### Scenario 3: OPC-UA PLC Integration

```yaml
node_configs:
  opcua:
    ENABLE_OPCUA: true
    OPCUA_URL: "opc.tcp://plc-1.factory.local:4840"
    OPCUA_NODE: "ns=2;s=DataSet"
    OPCUA_FREQUENCY: "5 seconds"

  operator:
    DEFAULT_DBMS: factory_data
```

### Scenario 4: Multi-Operator Cluster

Deploy multiple operators in the same cluster:

**operator-1-values.yaml:**
```yaml
metadata:
  app_name: operator-node-1

node_configs:
  general:
    NODE_NAME: operator-node-1
  networking:
    ANYLOG_SERVER_PORT: 32148
    ANYLOG_REST_PORT: 32149
  operator:
    MEMBER: "1"
```

**operator-2-values.yaml:**
```yaml
metadata:
  app_name: operator-node-2

node_configs:
  general:
    NODE_NAME: operator-node-2
  networking:
    ANYLOG_SERVER_PORT: 32248
    ANYLOG_REST_PORT: 32249
  operator:
    MEMBER: "2"
```

Install both:
```bash
helm install operator-1 ./edgelake-operator -f operator-1-values.yaml
helm install operator-2 ./edgelake-operator -f operator-2-values.yaml
```

## Post-Installation

### Access the Operator REST API

**Using NodePort:**
```bash
# Get node IP
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="ExternalIP")].address}')

# Get REST port
REST_PORT=$(kubectl get svc edgelake-operator-service -o jsonpath='{.spec.ports[?(@.name=="rest-api")].nodePort}')

# Test connection
curl http://$NODE_IP:$REST_PORT
```

**Using Port Forwarding (Development):**
```bash
kubectl port-forward svc/edgelake-operator-service 32149:32149 &
curl http://localhost:32149
```

### Register Operator Policy

Once the operator is running, register it with the master node:

```bash
# Access operator CLI
kubectl exec -it <operator-pod> -- /bin/bash

# Inside the pod, register operator policy
# (Commands depend on your setup)
```

### Monitor Operator

```bash
# View real-time logs
kubectl logs -f <operator-pod>

# Check resource usage
kubectl top pod <operator-pod>

# Get operator status
kubectl exec -it <operator-pod> -- curl localhost:32149/status
```

## Upgrading

```bash
# Upgrade with new values
helm upgrade operator-node-1 ./edgelake-operator \
  -f production-values.yaml \
  --namespace edgelake-prod

# Check upgrade status
helm status operator-node-1 -n edgelake-prod

# Rollback if needed
helm rollback operator-node-1 -n edgelake-prod
```

## Uninstalling

```bash
# Uninstall release
helm uninstall operator-node-1 -n edgelake-prod

# Optionally delete PVCs (this will delete data!)
kubectl delete pvc -n edgelake-prod -l app=edgelake-operator

# Delete namespace
kubectl delete namespace edgelake-prod
```

## Troubleshooting

### Pod Not Starting

```bash
# Describe pod to see events
kubectl describe pod <pod-name>

# Check logs
kubectl logs <pod-name>

# Common issues:
# 1. ImagePullBackOff: Check image name and pull secrets
# 2. CrashLoopBackOff: Check logs for configuration errors
# 3. Pending: Check PVC status or resource availability
```

### Cannot Connect to Master Node

```bash
# Test network connectivity from pod
kubectl exec -it <pod-name> -- curl -v <master-ip>:32048

# Check if LEDGER_CONN is correct
kubectl exec -it <pod-name> -- env | grep LEDGER_CONN

# Verify master node is reachable
kubectl exec -it <pod-name> -- ping <master-ip>
```

### Database Connection Issues

```bash
# For PostgreSQL, test connection
kubectl exec -it <pod-name> -- nc -zv <postgres-host> 5432

# Check database credentials in configmap
kubectl get configmap edgelake-operator-configmap -o yaml

# View database-related logs
kubectl logs <pod-name> | grep -i database
```

### Storage Issues

```bash
# Check PVC status
kubectl get pvc

# Describe PVC for events
kubectl describe pvc edgelake-operator-data-pvc

# Check storage class
kubectl get storageclass

# View pod volume mounts
kubectl describe pod <pod-name> | grep -A 10 Mounts
```

## Support

For additional support:
- Documentation: https://github.com/EdgeLake/EdgeLake
- Issues: https://github.com/EdgeLake/EdgeLake/issues
- Community: EdgeLake community channels
