# Open Horizon EdgeLake Deployment Plan

**Date:** 2025-11-17
**Author:** Infrastructure Planning

---

## Current Infrastructure Summary

### OH Hub Server (<user>@<hub-tailscale-ip>)
**Components Running:**
- Open Horizon Exchange
- Open Horizon CSS (Sync Service)
- Open Horizon AgBot (Agreement Bot)
- EdgeLake Master node
- EdgeLake Query node
- Docker Registry (for EdgeLake builds)
- Artifactory (unused currently)

### Existing Edge Node (<user>@<edge-srv-012-tailscale-ip>)
- EdgeLake Operator running (manual deployment)

---

## Objectives

1. Understand how demo-in-a-box-edgelake deploys operators via Open Horizon on Docker devices
2. Deploy EdgeLake operator to new Docker-based devices using Open Horizon
3. Deploy EdgeLake operator to Kubernetes cluster using Open Horizon + Helm charts

---

## Path 1: Docker-based Operator Deployment via Open Horizon

This mirrors what demo-in-a-box-edgelake does for VMs, adapted for your infrastructure.

### Understanding the OH Service Components

From the demo-in-a-box-edgelake analysis, an OH EdgeLake service consists of:

#### 1. Service Definition (`service.definition.json`)
- Defines the EdgeLake container deployment
- Specifies userInput parameters (NODE_TYPE, NODE_NAME, ports, database, etc.)
- Image: Uses variables like `$DOCKER_IMAGE_BASE:$DOCKER_IMAGE_VERSION`
- Network mode: `host`
- Volumes: 4 persistent volumes (anylog, blockchain, data, local-scripts)
- **Key requirement**: `privileged: true`

**Key userInput parameters:**
```json
{
  "ANYLOG_PATH": "/app",
  "NODE_TYPE": "operator",
  "NODE_NAME": "edgelake-operator",
  "COMPANY_NAME": "New Company",
  "ANYLOG_SERVER_PORT": 32148,
  "ANYLOG_REST_PORT": 32149,
  "LEDGER_CONN": "127.0.0.1:32048",
  "DB_TYPE": "sqlite",
  "CLUSTER_NAME": "",
  "DEFAULT_DBMS": "new_company",
  "ENABLE_MQTT": false,
  "MONITOR_NODES": false
}
```

#### 2. Service Policy (`service.policy.json`)
Very simple - just constraints:
```json
{
  "properties": [],
  "constraints": [
    "openhorizon.allowPrivileged == true AND purpose == edgelake"
  ]
}
```

#### 3. Deployment Policy (`service.deployment.json`)
- Maps service to nodes with specific constraints
- Defines default userInput values for specific deployment (e.g., query node)
- Example constraints: `purpose == edgelake` AND `openhorizon.allowPrivileged == true`

**Key fields:**
```json
{
  "label": "EdgeLake Deployment Policy",
  "service": {
    "name": "service-edgelake-operator",
    "org": "myorg",
    "serviceVersions": [{"version": "1.3.5"}]
  },
  "constraints": [
    "purpose == edgelake",
    "openhorizon.allowPrivileged == true"
  ],
  "userInput": [
    // Override default service values for this deployment
  ]
}
```

#### 4. Node Policy (`node.policy.json`)
Placed on edge devices to match with deployment policies:
```json
{
  "properties": [
    {"name": "purpose", "value": "edgelake"},
    {"name": "openhorizon.allowPrivileged", "value": true}
  ],
  "constraints": []
}
```

---

### Steps to Deploy EdgeLake Operator via OH to Docker Device

#### Prerequisites on New Device

Install required components:
```bash
# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER

# Install Open Horizon agent
# Method varies by OH version - typically:
wget http://<hub-tailscale-ip>/agent-install.sh
chmod +x agent-install.sh
sudo -s ./agent-install.sh -i 'css:' -p IBM/pattern-ibm.helloworld -w '*' -T 120
```

#### Step 1: Prepare Service Definition for Your Registry

Navigate to demo-in-a-box-edgelake and set environment:

```bash
cd /path/to/demo-in-a-box-edgelake

# Set your environment
export HZN_ORG_ID=myorg  # or your org name
export HZN_EXCHANGE_USER_AUTH=admin:<your-admin-password>
export EDGELAKE_TYPE=operator
export DOCKER_IMAGE_BASE=<hub-tailscale-ip>:5000/edgelake-mcp  # Your registry
export DOCKER_IMAGE_VERSION=amd64-latest  # Your image tag
export SERVICE_VERSION=1.3.5
export ARCH=amd64
```

#### Step 2: Customize Operator Configuration

Edit `edgelake/configurations/edgelake_operator.env`:

```bash
# Essential settings
NODE_TYPE=operator
NODE_NAME=edgelake-operator-<unique-name>
COMPANY_NAME="Your Company"

# Networking
ANYLOG_SERVER_PORT=32148
ANYLOG_REST_PORT=32149
TCP_BIND=false
REST_BIND=false

# Blockchain/Master connection
LEDGER_CONN=<hub-tailscale-ip>:32048  # Your master node
BLOCKCHAIN_SOURCE=master
BLOCKCHAIN_SYNC=30 second

# Database
DB_TYPE=sqlite  # or psql if using PostgreSQL
DB_IP=127.0.0.1
DB_PORT=5432

# Operator settings
CLUSTER_NAME=production-cluster
DEFAULT_DBMS=production_data
ENABLE_PARTITIONS=true
PARTITION_INTERVAL=14 days
PARTITION_KEEP=3

# Network overlay (if using Tailscale/Nebula)
OVERLAY_IP=""  # Set if using VPN

# Monitoring
MONITOR_NODES=true
```

#### Step 3: Update Service Definition for Your Registry

Edit `edgelake/service.definition.json` to use your registry:

```bash
# Update image reference
sed -i 's|anylogco/edgelake|<hub-tailscale-ip>:5000/edgelake-mcp|g' edgelake/service.definition.json
```

Or manually ensure the deployment section uses:
```json
{
  "deployment": {
    "services": {
      "$SERVICE_NAME": {
        "image": "<hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest",
        "network": "host",
        "privileged": true,
        "binds": [
          "edgelake-operator-anylog:/app/EdgeLake/anylog",
          "edgelake-operator-blockchain:/app/EdgeLake/blockchain",
          "edgelake-operator-data:/app/EdgeLake/data",
          "edgelake-operator-local-scripts:/app/deployment-scripts"
        ]
      }
    }
  }
}
```

#### Step 4: Generate and Publish Service

```bash
# Generate deployment policy with your configs
python edgelake/create_policy.py $SERVICE_VERSION edgelake/configurations/edgelake_operator.env

# Verify generated files look correct
cat edgelake/service.deployment.json

# Publish service definition to Exchange
hzn exchange service publish \
  --org=$HZN_ORG_ID \
  --user-pw=$HZN_EXCHANGE_USER_AUTH \
  -O -P \
  --json-file=edgelake/service.definition.json

# Verify service published
hzn exchange service list --org=$HZN_ORG_ID

# Add service policy
hzn exchange service addpolicy \
  --org=$HZN_ORG_ID \
  --user-pw=$HZN_EXCHANGE_USER_AUTH \
  -f edgelake/service.policy.json \
  $HZN_ORG_ID/service-edgelake-operator_${SERVICE_VERSION}_${ARCH}

# Publish deployment policy
hzn exchange deployment addpolicy \
  --org=$HZN_ORG_ID \
  --user-pw=$HZN_EXCHANGE_USER_AUTH \
  -f edgelake/service.deployment.json \
  $HZN_ORG_ID/policy-service-edgelake-operator_${SERVICE_VERSION}

# Verify policies published
hzn exchange deployment listpolicy --org=$HZN_ORG_ID
```

#### Step 5: Register Edge Device

On the new device (e.g., new device or re-register <edge-srv-012-tailscale-ip>):

```bash
# Set credentials
export HZN_ORG_ID=myorg
export HZN_EXCHANGE_USER_AUTH=admin:<your-admin-password>
export HZN_EXCHANGE_URL=http://<hub-tailscale-ip>:3090/v1
export HZN_FSS_CSSURL=http://<hub-tailscale-ip>:9443

# Create node policy file
cat > node-policy.json << EOF
{
  "properties": [
    {"name": "purpose", "value": "edgelake"},
    {"name": "openhorizon.allowPrivileged", "value": true},
    {"name": "node-location", "value": "datacenter-1"}
  ],
  "constraints": []
}
EOF

# Register node
hzn register \
  --org=$HZN_ORG_ID \
  --user-pw=$HZN_EXCHANGE_USER_AUTH \
  --name=edgelake-operator-device-1 \
  --policy=node-policy.json

# Monitor agreement formation
watch hzn agreement list

# Check service deployment
hzn service list

# Verify container running
docker ps | grep edgelake

# Check node status
hzn node list
```

#### Step 6: Verify Operator Deployment

```bash
# Get container name
CONTAINER_NAME=$(docker ps --filter "ancestor=<hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest" --format "{{.Names}}")

# Check EdgeLake logs
docker logs -f $CONTAINER_NAME

# Test REST API
curl http://localhost:32149/status

# Check connectivity to master
curl http://<hub-tailscale-ip>:32049/status

# Access EdgeLake CLI (if not disabled)
docker attach $CONTAINER_NAME
# Press Ctrl-D to detach (not Ctrl-C)

# Check volumes
docker volume ls | grep edgelake

# Inspect node policy
hzn policy list
```

#### Step 7: Troubleshooting

**No agreement formed:**
```bash
# Check node status
hzn node list

# Check eventlog for issues
hzn eventlog list

# Verify policies match
hzn exchange node listpolicy
hzn exchange deployment listpolicy --org=$HZN_ORG_ID

# Check agbot status on hub
ssh <user>@<hub-tailscale-ip> "hzn agbot agreement list"
```

**Container not starting:**
```bash
# Check Docker logs
docker logs $CONTAINER_NAME

# Verify image is accessible
docker pull <hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest

# Check if privileged mode is allowed
docker inspect $CONTAINER_NAME | grep -i privileged
```

**Network issues:**
```bash
# Test connectivity to master
telnet <hub-tailscale-ip> 32048
telnet <hub-tailscale-ip> 32049

# Check if ports are bound
netstat -tlnp | grep 3214

# Verify host network mode
docker inspect $CONTAINER_NAME | grep -i networkmode
```

---

## Path 2: Kubernetes-based Operator Deployment via Open Horizon

This uses Open Horizon's Kubernetes operator support with your Helm charts.

### Understanding OH + Kubernetes Integration

Open Horizon can deploy to Kubernetes clusters using:
- **Cluster agent**: OH agent running at cluster level
- **Operator pattern**: Uses Kubernetes custom resources
- **Service definition**: Points to Helm charts or k8s manifests

### Key Differences from Docker Deployment

1. **Service Definition Changes:**
   - Instead of Docker deployment section, use `clusterDeployment`
   - Reference Kubernetes manifests or Helm charts
   - Network mode handled by Kubernetes (no host networking by default)

2. **Architecture:**
   - OH agent runs as cluster-level service
   - Creates Kubernetes resources (Deployments, Services, PVCs)
   - Monitors health via k8s API

3. **Deployment Approach Options:**

   **Option A: OH-native Kubernetes Service**
   - Service definition contains k8s manifests directly
   - OH manages lifecycle via cluster agent

   **Option B: Hybrid Approach (Recommended)**
   - Use OH to trigger deployment
   - Service executes `helm install` command
   - Leverage your existing helm charts at `/path/to/utilities/edgelake/helm-charts`

---

### Steps for Kubernetes Deployment

#### Step 1: Install OH Cluster Agent

On your Kubernetes cluster:

```bash
# Create namespace for OH agent
kubectl create namespace openhorizon-agent

# Install OH agent operator (method varies by OH version)
# Option A: Using Helm (if available)
helm repo add openhorizon https://openhorizon.github.io/helm-charts
helm install agent-operator openhorizon/agent-operator \
  -n openhorizon-agent \
  --set horizon.exchange.url=http://<hub-tailscale-ip>:3090/v1 \
  --set horizon.css.url=http://<hub-tailscale-ip>:9443 \
  --set horizon.organization=$HZN_ORG_ID

# Option B: Manual installation
# Download agent operator manifests from your OH hub
wget http://<hub-tailscale-ip>/agent-operator-install.yaml
kubectl apply -f agent-operator-install.yaml

# Configure agent credentials
kubectl create secret generic openhorizon-agent-secrets \
  --from-literal=HZN_EXCHANGE_URL=http://<hub-tailscale-ip>:3090/v1 \
  --from-literal=HZN_FSS_CSSURL=http://<hub-tailscale-ip>:9443 \
  --from-literal=HZN_ORG_ID=myorg \
  --from-literal=HZN_EXCHANGE_USER_AUTH=admin:<password> \
  -n openhorizon-agent

# Verify agent is running
kubectl get pods -n openhorizon-agent
kubectl logs -n openhorizon-agent -l app=agent-operator
```

#### Step 2: Create Helm-based OH Service

Create `edgelake-k8s-service.json`:

```json
{
  "org": "$HZN_ORG_ID",
  "label": "EdgeLake Operator for Kubernetes",
  "description": "EdgeLake Operator deployed via Helm on Kubernetes",
  "url": "service-edgelake-operator-k8s",
  "version": "1.0.0",
  "arch": "amd64",
  "public": true,
  "sharable": "multiple",
  "requiredServices": [],
  "userInput": [
    {
      "name": "LEDGER_CONN",
      "label": "Master node connection",
      "type": "string",
      "defaultValue": "<hub-tailscale-ip>:32048"
    },
    {
      "name": "NODE_NAME",
      "label": "Operator node name",
      "type": "string",
      "defaultValue": "operator-k8s-1"
    },
    {
      "name": "CLUSTER_NAME",
      "label": "EdgeLake cluster name",
      "type": "string",
      "defaultValue": "k8s-cluster1"
    },
    {
      "name": "COMPANY_NAME",
      "label": "Company name",
      "type": "string",
      "defaultValue": "New Company"
    },
    {
      "name": "DEFAULT_DBMS",
      "label": "Default database name",
      "type": "string",
      "defaultValue": "production_data"
    }
  ],
  "clusterDeployment": {
    "operatorYamlArchive": "edgelake-operator-helm.tar.gz"
  }
}
```

#### Step 3: Package Helm Chart for OH

```bash
cd /path/to/utilities/edgelake/helm-charts

# Create values override for OH deployment
cat > oh-operator-values.yaml << EOF
metadata:
  namespace: edgelake
  app_name: operator-k8s-1
  service_type: NodePort
  node_selector: ""

image:
  repository: <hub-tailscale-ip>:5000/edgelake-mcp
  tag: amd64-latest
  pull_policy: IfNotPresent
  secret_name: ""

persistence:
  enabled: true
  storageClassName: ""
  anylog:
    size: 5Gi
  blockchain:
    size: 1Gi
  data:
    size: 20Gi
  scripts:
    size: 1Gi

resources:
  limits:
    cpu: "2000m"
    memory: "4Gi"
  requests:
    cpu: "500m"
    memory: "1Gi"

node_configs:
  general:
    NODE_NAME: operator-k8s-1
    COMPANY_NAME: "Your Company"
    DISABLE_CLI: false
    REMOTE_CLI: false

  networking:
    OVERLAY_IP: ""
    ANYLOG_SERVER_PORT: 32148
    ANYLOG_REST_PORT: 32149
    TCP_BIND: false
    REST_BIND: false

  blockchain:
    LEDGER_CONN: "<hub-tailscale-ip>:32048"
    BLOCKCHAIN_SOURCE: master
    BLOCKCHAIN_SYNC: "30 second"
    BLOCKCHAIN_DESTINATION: file

  database:
    DB_TYPE: sqlite
    DB_IP: 127.0.0.1
    DB_PORT: 5432
    AUTOCOMMIT: false
    SYSTEM_QUERY: false
    MEMORY: false

  operator:
    CLUSTER_NAME: k8s-cluster1
    DEFAULT_DBMS: production_data
    ENABLE_PARTITIONS: true
    PARTITION_INTERVAL: "14 days"
    PARTITION_KEEP: 3
    OPERATOR_THREADS: 3

  monitoring:
    MONITOR_NODES: true
    STORE_MONITORING: false

  advanced:
    COMPRESS_FILE: true
    QUERY_POOL: 6
    THRESHOLD_TIME: "60 seconds"
    THRESHOLD_VOLUME: "100KB"
EOF

# Package helm chart
helm package edgelake-operator

# Create operator archive for OH
# OH expects a tar.gz with Helm chart and install script
mkdir -p oh-package
cp edgelake-operator-*.tgz oh-package/
cp oh-operator-values.yaml oh-package/values.yaml

# Create install script
cat > oh-package/install.sh << 'EOF'
#!/bin/bash
set -e

# Extract chart
tar -xzf edgelake-operator-*.tgz

# Install or upgrade
helm upgrade --install operator-k8s-1 ./edgelake-operator \
  -f values.yaml \
  --namespace edgelake \
  --create-namespace \
  --wait \
  --timeout 10m

echo "EdgeLake operator deployed successfully"
kubectl get pods -n edgelake -l app=edgelake-operator
EOF

chmod +x oh-package/install.sh

# Create uninstall script
cat > oh-package/uninstall.sh << 'EOF'
#!/bin/bash
helm uninstall operator-k8s-1 -n edgelake || true
kubectl delete namespace edgelake || true
EOF

chmod +x oh-package/uninstall.sh

# Create final archive
cd oh-package
tar czf ../edgelake-operator-helm.tar.gz *
cd ..
```

#### Step 4: Publish to OH Exchange

```bash
# Set environment
export HZN_ORG_ID=myorg
export HZN_EXCHANGE_USER_AUTH=admin:<password>

# Publish the service with operator archive
hzn exchange service publish \
  --org=$HZN_ORG_ID \
  --user-pw=$HZN_EXCHANGE_USER_AUTH \
  -f edgelake-k8s-service.json \
  --operator-archive edgelake-operator-helm.tar.gz

# Verify service published
hzn exchange service list --org=$HZN_ORG_ID | grep k8s

# Create service policy
cat > edgelake-k8s-service-policy.json << EOF
{
  "properties": [],
  "constraints": [
    "openhorizon.cluster == true AND purpose == edgelake-k8s"
  ]
}
EOF

hzn exchange service addpolicy \
  --org=$HZN_ORG_ID \
  --user-pw=$HZN_EXCHANGE_USER_AUTH \
  -f edgelake-k8s-service-policy.json \
  $HZN_ORG_ID/service-edgelake-operator-k8s_1.0.0_amd64

# Create deployment policy
cat > k8s-deployment-policy.json << EOF
{
  "label": "EdgeLake K8s Operator Deployment",
  "description": "Deploy EdgeLake operator to Kubernetes",
  "service": {
    "name": "service-edgelake-operator-k8s",
    "org": "$HZN_ORG_ID",
    "arch": "*",
    "serviceVersions": [
      {
        "version": "1.0.0",
        "priority": {
          "priority_value": 2,
          "retries": 2,
          "retry_durations": 1800
        }
      }
    ]
  },
  "properties": [],
  "constraints": [
    "purpose == edgelake-k8s",
    "openhorizon.cluster == true"
  ],
  "userInput": [
    {
      "serviceOrgid": "$HZN_ORG_ID",
      "serviceUrl": "service-edgelake-operator-k8s",
      "serviceVersionRange": "[0.0.0,INFINITY)",
      "inputs": [
        {
          "name": "LEDGER_CONN",
          "value": "<hub-tailscale-ip>:32048"
        },
        {
          "name": "NODE_NAME",
          "value": "operator-k8s-1"
        },
        {
          "name": "CLUSTER_NAME",
          "value": "k8s-cluster1"
        },
        {
          "name": "COMPANY_NAME",
          "value": "Your Company"
        },
        {
          "name": "DEFAULT_DBMS",
          "value": "production_data"
        }
      ]
    }
  ]
}
EOF

hzn exchange deployment addpolicy \
  --org=$HZN_ORG_ID \
  --user-pw=$HZN_EXCHANGE_USER_AUTH \
  -f k8s-deployment-policy.json \
  $HZN_ORG_ID/policy-edgelake-k8s-operator

# Verify
hzn exchange deployment listpolicy --org=$HZN_ORG_ID
```

#### Step 5: Register Kubernetes Cluster with OH

On the cluster with OH agent installed:

```bash
# Create cluster node policy
cat > cluster-node-policy.json << EOF
{
  "properties": [
    {"name": "purpose", "value": "edgelake-k8s"},
    {"name": "openhorizon.cluster", "value": true},
    {"name": "openhorizon.kubernetesVersion", "value": "1.28"},
    {"name": "cluster-location", "value": "datacenter-1"}
  ],
  "constraints": []
}
EOF

# Register the cluster (typically done via kubectl)
# Method 1: Using hzn CLI if available in cluster
kubectl exec -it -n openhorizon-agent <agent-pod> -- \
  hzn register \
    --org=$HZN_ORG_ID \
    --user-pw=$HZN_EXCHANGE_USER_AUTH \
    --policy=cluster-node-policy.json

# Method 2: Using CRD (Custom Resource Definition)
cat > cluster-registration.yaml << EOF
apiVersion: openhorizon.github.io/v1
kind: AgentRegistration
metadata:
  name: edgelake-cluster
  namespace: openhorizon-agent
spec:
  organization: $HZN_ORG_ID
  policy: |
    {
      "properties": [
        {"name": "purpose", "value": "edgelake-k8s"},
        {"name": "openhorizon.cluster", "value": true}
      ]
    }
EOF

kubectl apply -f cluster-registration.yaml
```

#### Step 6: Verify Deployment

```bash
# Check OH agreement (from cluster)
kubectl exec -n openhorizon-agent <agent-pod> -- hzn agreement list

# Check Kubernetes resources
kubectl get all -n edgelake

# Check pods
kubectl get pods -n edgelake -l app=edgelake-operator

# Check services
kubectl get svc -n edgelake

# Check PVCs
kubectl get pvc -n edgelake

# View logs
kubectl logs -n edgelake -l app=edgelake-operator -f

# Check ConfigMap
kubectl get configmap -n edgelake edgelake-operator-configmap -o yaml

# Test EdgeLake API via port-forward
kubectl port-forward -n edgelake svc/edgelake-operator-service 32149:32149 &
curl http://localhost:32149/status

# Or via NodePort (if service_type: NodePort)
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="ExternalIP")].address}')
NODE_PORT=$(kubectl get svc -n edgelake edgelake-operator-service -o jsonpath='{.spec.ports[?(@.name=="rest-api")].nodePort}')
curl http://$NODE_IP:$NODE_PORT/status

# Check connectivity to master
kubectl exec -n edgelake <operator-pod> -- curl -v <hub-tailscale-ip>:32048
```

#### Step 7: Troubleshooting Kubernetes Deployment

**OH agent issues:**
```bash
# Check agent logs
kubectl logs -n openhorizon-agent -l app=agent-operator

# Check agent status
kubectl get pods -n openhorizon-agent

# Verify agent can reach exchange
kubectl exec -n openhorizon-agent <agent-pod> -- curl http://<hub-tailscale-ip>:3090/v1/admin/version
```

**Service not deploying:**
```bash
# Check agreement status
kubectl exec -n openhorizon-agent <agent-pod> -- hzn agreement list

# Check eventlog
kubectl exec -n openhorizon-agent <agent-pod> -- hzn eventlog list

# Verify operator archive uploaded
hzn exchange service list --org=$HZN_ORG_ID -l | grep k8s
```

**Helm deployment failing:**
```bash
# Check Helm releases
kubectl exec -n openhorizon-agent <agent-pod> -- helm list -A

# Check install script execution
kubectl logs -n openhorizon-agent <agent-pod> | grep -i install

# Manual test of Helm chart
helm install test-operator /path/to/edgelake-operator \
  -f oh-operator-values.yaml \
  --namespace edgelake-test \
  --create-namespace \
  --dry-run --debug
```

**Operator pod issues:**
```bash
# Describe pod for events
kubectl describe pod -n edgelake <operator-pod>

# Check init containers
kubectl logs -n edgelake <operator-pod> -c <init-container>

# Check image pull
kubectl describe pod -n edgelake <operator-pod> | grep -i image

# Check resource constraints
kubectl describe pod -n edgelake <operator-pod> | grep -A 10 Limits
```

**Network connectivity:**
```bash
# Test master connectivity from pod
kubectl exec -n edgelake <operator-pod> -- nc -zv <hub-tailscale-ip> 32048
kubectl exec -n edgelake <operator-pod> -- nc -zv <hub-tailscale-ip> 32049

# Check service endpoints
kubectl get endpoints -n edgelake

# Test DNS resolution
kubectl exec -n edgelake <operator-pod> -- nslookup <hub-tailscale-ip>
```

---

## Alternative: Manual Helm + OH Monitoring (Simpler Approach)

If full OH Kubernetes integration proves complex, consider this hybrid approach:

### Approach: Deploy via Helm, Monitor via OH

**Advantages:**
- Use proven Helm charts directly
- OH provides monitoring and lifecycle management
- Simpler initial setup

**Process:**

1. **Deploy EdgeLake Operator via Helm (manual):**
```bash
helm install operator-1 /path/to/utilities/edgelake/helm-charts/edgelake-operator \
  -f production-values.yaml \
  -n edgelake \
  --create-namespace
```

2. **Create OH monitoring service:**
```json
{
  "org": "myorg",
  "label": "EdgeLake K8s Monitor",
  "url": "service-edgelake-k8s-monitor",
  "version": "1.0.0",
  "arch": "amd64",
  "deployment": {
    "services": {
      "monitor": {
        "image": "alpine:latest",
        "command": ["/bin/sh", "-c"],
        "args": [
          "while true; do kubectl get pods -n edgelake -l app=edgelake-operator; sleep 60; done"
        ],
        "binds": [
          "/home/user/.kube/config:/root/.kube/config:ro"
        ]
      }
    }
  }
}
```

3. **OH reports status but doesn't manage deployment**
   - Service runs health checks
   - Reports to OH Exchange
   - Alerts on failures
   - Doesn't control lifecycle

---

## Recommended Implementation Path

### Phase 1: Docker Deployment (Week 1)
1. **Day 1-2:** Set up service definitions
   - Customize demo-in-a-box configs for your registry
   - Test service publishing to Exchange

2. **Day 3-4:** Deploy to existing operator (<edge-srv-012-tailscale-ip>)
   - Unregister current manual deployment
   - Register with OH
   - Verify deployment works

3. **Day 5:** Deploy to new device
   - Install OH agent on new device
   - Register and verify automatic deployment
   - Document process

### Phase 2: Kubernetes Deployment (Week 2-3)
1. **Week 2:** Infrastructure setup
   - Install OH cluster agent
   - Test agent connectivity to hub
   - Create test deployments

2. **Week 3:** EdgeLake deployment
   - Package Helm charts for OH
   - Publish service to Exchange
   - Deploy and verify
   - Document process

### Phase 3: Production Rollout (Week 4)
1. **Create templates** for both deployment methods
2. **Document procedures** for adding new nodes
3. **Set up monitoring** and alerting
4. **Train team** on OH management

---

## Key Files and Locations

### Demo-in-a-box Repository
- **Location:** `/path/to/demo-in-a-box-edgelake`
- **Service definitions:** `edgelake/service.definition.json`
- **Deployment policy:** `edgelake/service.deployment.json`
- **Node policy:** `edgelake/node.policy.json`
- **Configurations:** `edgelake/configurations/edgelake_*.env`
- **Makefile:** `Makefile` (has targets for publishing services)

### Helm Charts
- **Location:** `/path/to/utilities/edgelake/helm-charts/edgelake-operator`
- **Chart:** `Chart.yaml`
- **Values:** `values.yaml`
- **Templates:** `templates/`
- **Install guide:** `INSTALL.md`

### Infrastructure Config
- **Location:** `/path/to/trv-edgelake-infra/trv-docker-compose`
- **Deployments:** `deployments/master`, `deployments/operator`, `deployments/query`
- **Makefile:** `Makefile` (for local Docker deployments)

---

## Next Steps

### Immediate Actions:
1. Review this plan and identify any gaps
2. Decide on Phase 1 implementation timeline
3. Verify OH hub credentials and access
4. Test registry access from edge devices

### Questions to Resolve:
1. What is your OH organization ID (HZN_ORG_ID)?
2. What are the Exchange admin credentials?
3. Do you want to test on <edge-srv-012-tailscale-ip> first or a new device?
4. Which Kubernetes cluster will you use for Phase 2?
5. Do you want PostgreSQL or SQLite for production operators?

### Resources Needed:
- OH hub access credentials
- Edge device access (SSH)
- Kubernetes cluster access (kubectl)
- Docker registry access from edge nodes
- Master node connection details confirmed

---

## Support and References

### EdgeLake Documentation
- **Location:** `/path/to/documentation`
- Main AnyLog/EdgeLake documentation

### Open Horizon Documentation
- Service definitions: https://open-horizon.github.io/
- Agent installation: https://open-horizon.github.io/quick-start/
- Kubernetes integration: https://open-horizon.github.io/docs/

### Internal Resources
- Docker registry: `<hub-tailscale-ip>:5000`
- Artifactory: `<hub-tailscale-ip>:<port>` (to be configured)
- OH Exchange: `http://<hub-tailscale-ip>:3090/v1`
- OH CSS: `http://<hub-tailscale-ip>:9443`

---

**End of Deployment Plan**
