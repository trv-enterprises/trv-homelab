# EdgeLake Operator Helm Chart

This Helm chart deploys an EdgeLake Operator node on Kubernetes.

## Overview

EdgeLake Operator nodes are responsible for:
- Capturing data from devices, PLCs, and applications
- Hosting data on a local DBMS (PostgreSQL, SQLite, or MongoDB)
- Handling data ingestion from various sources (MQTT, REST, gRPC, OPC-UA, EtherNet/IP)
- Participating in distributed query execution
- Managing data partitioning and retention

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- Persistent Volume provisioner support in the underlying infrastructure (if using persistent storage)
- Access to EdgeLake Docker image (public or private registry)

## Installation

### Quick Start

Install with default values:

```bash
helm install my-operator edgelake-operator/
```

### Custom Installation

1. Create a custom values file:

```bash
cp values.yaml my-values.yaml
```

2. Edit `my-values.yaml` with your configuration:

```yaml
metadata:
  namespace: edgelake
  app_name: my-operator

node_configs:
  general:
    COMPANY_NAME: "My Company"

  blockchain:
    LEDGER_CONN: "master-node-ip:32048"

  operator:
    CLUSTER_NAME: my-cluster
    DEFAULT_DBMS: my_database
```

3. Install with custom values:

```bash
helm install my-operator edgelake-operator/ -f my-values.yaml
```

### Install in specific namespace

```bash
kubectl create namespace edgelake
helm install my-operator edgelake-operator/ --namespace edgelake
```

## Configuration

### Key Configuration Sections

#### Blockchain/Master Node Connection

**REQUIRED**: Configure the master node connection:

```yaml
node_configs:
  blockchain:
    LEDGER_CONN: "master-ip:32048"
```

#### Operator Configuration

```yaml
node_configs:
  operator:
    CLUSTER_NAME: my-cluster
    DEFAULT_DBMS: my_database
    ENABLE_PARTITIONS: true
    PARTITION_INTERVAL: "14 days"
```

#### Database Backend

SQLite (default, no external database required):
```yaml
node_configs:
  database:
    DB_TYPE: sqlite
```

PostgreSQL (recommended for production):
```yaml
node_configs:
  database:
    DB_TYPE: psql
    DB_USER: edgelake
    DB_PASSWD: password
    DB_IP: postgres-service
    DB_PORT: 5432
```

#### MQTT Data Ingestion

```yaml
node_configs:
  mqtt:
    ENABLE_MQTT: true
    MQTT_BROKER: mqtt-broker-ip
    MQTT_PORT: 1883
    MQTT_USER: username
    MQTT_PASSWD: password
    MSG_TOPIC: sensor-data
```

#### OPC-UA Integration

```yaml
node_configs:
  opcua:
    ENABLE_OPCUA: true
    OPCUA_URL: "opc.tcp://plc-ip:4840"
    OPCUA_NODE: "ns=2;s=DataSet"
    OPCUA_FREQUENCY: "5 seconds"
```

#### Persistent Storage

```yaml
persistence:
  enabled: true
  storageClassName: "standard"
  anylog:
    size: 5Gi
  blockchain:
    size: 1Gi
  data:
    size: 10Gi
```

#### Resource Limits

```yaml
resources:
  limits:
    cpu: "2000m"
    memory: "4Gi"
  requests:
    cpu: "500m"
    memory: "1Gi"
```

#### Service Type

ClusterIP (internal only):
```yaml
metadata:
  service_type: ClusterIP
```

NodePort (external access):
```yaml
metadata:
  service_type: NodePort
```

LoadBalancer (cloud environments):
```yaml
metadata:
  service_type: LoadBalancer
```

## Accessing the Operator

### Port Forwarding (Development)

```bash
# Forward REST API port
kubectl port-forward svc/edgelake-operator-service 32149:32149

# Forward TCP server port
kubectl port-forward svc/edgelake-operator-service 32148:32148
```

### NodePort (Production)

Access via any Kubernetes node IP:
```
http://<node-ip>:32149  # REST API
tcp://<node-ip>:32148   # TCP Server
```

### Check Status

```bash
# Get pod status
kubectl get pods -l app=edgelake-operator

# View logs
kubectl logs -l app=edgelake-operator -f

# Execute commands in pod
kubectl exec -it <pod-name> -- /bin/bash
```

## Upgrading

```bash
helm upgrade my-operator edgelake-operator/ -f my-values.yaml
```

## Uninstalling

```bash
helm uninstall my-operator
```

Note: By default, PersistentVolumeClaims are not deleted. To delete them:

```bash
kubectl delete pvc -l app.kubernetes.io/name=edgelake-operator
```

## Common Configurations

### Production Operator with PostgreSQL

```yaml
metadata:
  service_type: NodePort

node_configs:
  blockchain:
    LEDGER_CONN: "10.0.0.10:32048"

  database:
    DB_TYPE: psql
    DB_USER: edgelake
    DB_PASSWD: securepassword
    DB_IP: postgres-service
    DB_PORT: 5432

  operator:
    CLUSTER_NAME: production-cluster
    DEFAULT_DBMS: production_db
    ENABLE_PARTITIONS: true
    PARTITION_INTERVAL: "7 days"
    PARTITION_KEEP: 4

persistence:
  enabled: true
  storageClassName: "fast-ssd"
  data:
    size: 100Gi

resources:
  limits:
    cpu: "4000m"
    memory: "8Gi"
  requests:
    cpu: "1000m"
    memory: "2Gi"
```

### Development Operator (SQLite, no persistence)

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
  requests:
    cpu: "250m"
    memory: "512Mi"
```

## Troubleshooting

### Pod not starting

```bash
kubectl describe pod <pod-name>
kubectl logs <pod-name>
```

### Cannot connect to master node

Check `LEDGER_CONN` configuration and network policies:
```bash
kubectl exec -it <pod-name> -- curl -v master-ip:32048
```

### Database connection issues

For PostgreSQL, verify database service is accessible:
```bash
kubectl exec -it <pod-name> -- nc -zv postgres-service 5432
```

### Storage issues

Check PVC status:
```bash
kubectl get pvc
kubectl describe pvc edgelake-operator-data-pvc
```

## Values Reference

See [values.yaml](values.yaml) for complete configuration options.

## Support

- Documentation: https://github.com/EdgeLake/EdgeLake
- Issues: https://github.com/EdgeLake/EdgeLake/issues

## License

Mozilla Public License 2.0
