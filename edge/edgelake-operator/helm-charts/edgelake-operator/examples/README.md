# EdgeLake Operator Configuration Examples

This directory contains example values files for common EdgeLake Operator deployment scenarios.

## Available Examples

### 1. production-psql.yaml
**Production deployment with PostgreSQL**
- PostgreSQL database backend
- Persistent storage with custom storage class
- High resource allocation (4 CPU, 8GB RAM)
- Data partitioning enabled (7 days)
- High availability enabled
- Monitoring enabled

**Use case:** Production environments with high data volume

**Install:**
```bash
helm install operator-prod ./edgelake-operator -f examples/production-psql.yaml
```

### 2. development.yaml
**Development/testing deployment**
- SQLite database (no external DB required)
- No persistent storage (ephemeral)
- Minimal resource allocation (1 CPU, 2GB RAM)
- Debug mode enabled
- No partitioning

**Use case:** Local development, testing, proof-of-concept

**Install:**
```bash
helm install operator-dev ./edgelake-operator -f examples/development.yaml
```

### 3. mqtt-ingestion.yaml
**MQTT data ingestion**
- MQTT client enabled
- Configured to connect to MQTT broker
- Dynamic table mapping from messages
- MQTT broker port exposed
- Immediate write enabled for real-time data

**Use case:** IoT deployments with MQTT message brokers

**Configuration required:**
- Update `MQTT_BROKER` with your broker address
- Update `MQTT_USER` and `MQTT_PASSWD` with credentials
- Adjust `MSG_TOPIC` for your topic structure

**Install:**
```bash
helm install operator-mqtt ./edgelake-operator -f examples/mqtt-ingestion.yaml
```

### 4. opcua-plc.yaml
**OPC-UA PLC integration**
- OPC-UA client enabled
- Configured to poll PLC data
- PostgreSQL backend for industrial data
- Geolocation configured
- Monitoring enabled

**Use case:** Manufacturing/industrial environments with OPC-UA PLCs

**Configuration required:**
- Update `OPCUA_URL` with your PLC address
- Update `OPCUA_NODE` with the data root node
- Adjust `OPCUA_FREQUENCY` for polling interval

**Install:**
```bash
helm install operator-plc ./edgelake-operator -f examples/opcua-plc.yaml
```

## Customizing Examples

All examples can be customized for your specific needs:

### 1. Copy example file
```bash
cp examples/production-psql.yaml my-config.yaml
```

### 2. Edit configuration
```bash
# Update key values:
# - metadata.namespace
# - metadata.app_name
# - node_configs.general.COMPANY_NAME
# - node_configs.blockchain.LEDGER_CONN (master node address)
# - Database credentials
# - Storage sizes
# - Resource limits
```

### 3. Install with custom config
```bash
helm install my-operator ./edgelake-operator -f my-config.yaml
```

## Common Customizations

### Change Master Node Connection
```yaml
node_configs:
  blockchain:
    LEDGER_CONN: "your-master-ip:32048"
```

### Adjust Storage Sizes
```yaml
persistence:
  data:
    size: 200Gi  # Increase data volume
  anylog:
    size: 20Gi
```

### Modify Resource Limits
```yaml
resources:
  limits:
    cpu: "8000m"
    memory: "16Gi"
  requests:
    cpu: "2000m"
    memory: "4Gi"
```

### Enable Additional Features
```yaml
node_configs:
  monitoring:
    MONITOR_NODES: true
    STORE_MONITORING: true

  mcp:
    MCP_AUTOSTART: true

  operator:
    ENABLE_HA: true
```

## Combining Multiple Features

You can combine configurations from different examples. For instance, to create an operator with both MQTT and OPC-UA:

```yaml
node_configs:
  mqtt:
    ENABLE_MQTT: true
    MQTT_BROKER: "mqtt-broker:1883"
    # ... MQTT config

  opcua:
    ENABLE_OPCUA: true
    OPCUA_URL: "opc.tcp://plc:4840"
    # ... OPC-UA config
```

## Security Considerations

For production deployments:

1. **Use Kubernetes Secrets for passwords:**
   ```bash
   kubectl create secret generic db-credentials \
     --from-literal=password='your-password'
   ```

2. **Reference secrets in deployment:**
   ```yaml
   # Add to deployment.yaml
   env:
   - name: DB_PASSWD
     valueFrom:
       secretKeyRef:
         name: db-credentials
         key: password
   ```

3. **Enable network policies:**
   ```bash
   kubectl apply -f network-policy.yaml
   ```

4. **Use RBAC:**
   ```bash
   kubectl apply -f rbac.yaml
   ```

## Testing Examples

After installing with an example configuration:

```bash
# Check pod status
kubectl get pods -l app=<app-name>

# View logs
kubectl logs -l app=<app-name> -f

# Test REST API
kubectl port-forward svc/<service-name> 32149:32149
curl http://localhost:32149/status

# Access CLI
kubectl exec -it <pod-name> -- /bin/bash
```

## Support

For questions about these examples:
- Review the main README.md
- Check INSTALL.md for detailed installation steps
- Visit https://github.com/EdgeLake/EdgeLake for documentation
