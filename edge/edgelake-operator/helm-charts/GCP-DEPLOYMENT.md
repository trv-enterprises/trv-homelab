# EdgeLake Operator Deployment on Google Kubernetes Engine (GKE)

This document analyzes deployment strategies for EdgeLake operator nodes on Google Kubernetes Engine (GKE).

## Overview

EdgeLake operators require direct TCP socket connectivity for cluster communication. Unlike traditional HTTP-based microservices, EdgeLake uses custom TCP protocols on specific ports, which affects networking design in Kubernetes.

## Current Architecture (On-Premises)

**Environment:** k3s on trv-srv-011
**Networking:** NodePort Service + Tailscale overlay network
**Image:** Private registry at `<hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest`

```
External Network (Tailscale: 100.74.102.38)
         ↓
k3s Host NodePort (32448, 32449, 32450)
         ↓
Service ClusterIP
         ↓
Pod (edgelake-operator-k8s)
```

**Key Configuration:**
- `OVERLAY_IP`: Tailscale IP of k3s host (100.74.102.38)
- `service_type`: NodePort
- Ports: 32448 (TCP), 32449 (REST), 32450 (MQTT)

## GKE Networking Options

### Option 1: TCP/UDP Load Balancer (L4 ILB) - **RECOMMENDED**

**Architecture:**
```
Internet
    ↓
Google Cloud TCP/UDP Load Balancer (Layer 4)
    ↓ (Backend: Instance Groups)
GKE Worker Nodes
    ↓
Service
    ↓
Pod (edgelake-operator)
```

**Helm Configuration:**
```yaml
metadata:
  service_type: LoadBalancer
  annotations:
    # Use TCP/UDP (Network) load balancer, not HTTP(S)
    cloud.google.com/load-balancer-type: "External"

    # Optional: Reserve static external IP first
    # gcloud compute addresses create edgelake-operator-ip --region=us-central1
    # cloud.google.com/load-balancer-ip: "34.123.45.67"

    # Session affinity (if needed for EdgeLake)
    service.spec.sessionAffinity: "ClientIP"
    service.spec.sessionAffinityConfig.clientIP.timeoutSeconds: "3600"

node_configs:
  networking:
    # Load Balancer IP - set after deployment or use reserved IP
    OVERLAY_IP: "34.123.45.67"
    ANYLOG_SERVER_PORT: 32448
    ANYLOG_REST_PORT: 32449
    ANYLOG_BROKER_PORT: 32450
```

**Post-Deployment Setup:**
```bash
# 1. Reserve static IP (optional but recommended)
gcloud compute addresses create edgelake-operator-ip \
  --region=us-central1

gcloud compute addresses describe edgelake-operator-ip \
  --region=us-central1 \
  --format="get(address)"
# Output: 34.123.45.67

# 2. Deploy with Helm (with reserved IP annotation)
helm install edgelake-operator ./edgelake-operator -f gcp-values.yaml

# 3. Get Load Balancer IP (if not using reserved IP)
LB_IP=$(kubectl get svc edgelake-operator-service -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo "Load Balancer IP: $LB_IP"

# 4. Update EdgeLake configuration with LB IP
# Either:
#   a) Upgrade Helm release with OVERLAY_IP set
#   b) Restart pod to pick up new config

# 5. Verify connectivity
curl http://$LB_IP:32449/get/status
```

**Advantages:**
- ✅ Fully managed Google Cloud service
- ✅ High availability across zones
- ✅ Layer 4 TCP load balancing (perfect for EdgeLake)
- ✅ Static IP option via reserved addresses
- ✅ Kubernetes-native (standard LoadBalancer service)
- ✅ Regional or global load balancing
- ✅ Health checks

**Considerations:**
- 💰 Cost: ~$18/month + data processing ($0.008/GB)
- 📝 External IP assignment delay (~2-3 minutes)
- 🔒 Firewall rules required for ingress

**Cost Estimate:**
- Load Balancer: $0.025/hour × 730 hours = ~$18.25/month
- Forwarding rules: $0.01/hour per rule × 3 ports × 730 = ~$21.90/month
- Data processing: $0.008-0.012/GB
- Total: ~$40-50/month depending on traffic

---

### Option 2: NodePort + Reserved External IP

**Architecture:**
```
Internet
    ↓
Reserved External IP (on GKE node)
    ↓
GKE Worker Node (ports 32448-32450)
    ↓
Service NodePort
    ↓
Pod (edgelake-operator)
```

**Helm Configuration:**
```yaml
metadata:
  service_type: NodePort

node_configs:
  networking:
    # External IP of GKE node
    OVERLAY_IP: "35.192.1.2"
    ANYLOG_SERVER_PORT: 32448
    ANYLOG_REST_PORT: 32449
    ANYLOG_BROKER_PORT: 32450
```

**Manual Setup Required:**
```bash
# 1. Create GKE cluster with specific node pool
gcloud container clusters create edgelake-cluster \
  --region=us-central1 \
  --num-nodes=1 \
  --machine-type=n1-standard-2 \
  --disk-size=50

# 2. Reserve external IP
gcloud compute addresses create edgelake-node-ip \
  --region=us-central1
# Output: 35.192.1.2

# 3. Get node name
NODE_NAME=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')

# 4. Get instance name from node
INSTANCE_NAME=$(gcloud compute instances list \
  --filter="name:${NODE_NAME}" \
  --format="get(name)")

# 5. Assign external IP to node instance
gcloud compute instances delete-access-config $INSTANCE_NAME \
  --zone=us-central1-a \
  --access-config-name="external-nat"

gcloud compute instances add-access-config $INSTANCE_NAME \
  --zone=us-central1-a \
  --access-config-name="external-nat" \
  --address=35.192.1.2

# 6. Create firewall rules for NodePort range
gcloud compute firewall-rules create edgelake-nodeports \
  --allow=tcp:32448,tcp:32449,tcp:32450 \
  --source-ranges=0.0.0.0/0 \
  --description="EdgeLake operator ports"

# 7. Deploy EdgeLake
helm install edgelake-operator ./edgelake-operator \
  --set node_configs.networking.OVERLAY_IP="35.192.1.2" \
  -f gcp-nodeport-values.yaml
```

**Advantages:**
- ✅ Lower cost (only IP: $7.30/month if unused, free if attached)
- ✅ Static IP address
- ✅ Simple architecture
- ✅ Direct node access

**Considerations:**
- ⚠️ Manual IP management
- ⚠️ Single point of failure (one node)
- ⚠️ Pod affinity required
- ⚠️ Node replacement breaks connectivity
- ⚠️ Firewall rule management
- ⚠️ Not HA

**When to Use:**
- Development/testing environments
- Cost-sensitive deployments
- Single-node clusters

---

### Option 3: HostNetwork Mode

**Architecture:**
```
Internet
    ↓
Reserved External IP (on GKE node)
    ↓
GKE Worker Node - Pod in host network namespace
    ↓
Pod binds directly to host ports 32448-32450
```

**Helm Configuration:**

Modify `deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      hostNetwork: true  # Pod uses host's network
      dnsPolicy: ClusterFirstWithHostNet
      containers:
      - name: edgelake-operator
        ports:
        - containerPort: 32448
          hostPort: 32448
        - containerPort: 32449
          hostPort: 32449
        - containerPort: 32450
          hostPort: 32450
```

**Values:**
```yaml
hostNetwork: true

node_configs:
  networking:
    OVERLAY_IP: "35.192.1.2"  # Node external IP
    ANYLOG_SERVER_PORT: 32448
    ANYLOG_REST_PORT: 32449
    ANYLOG_BROKER_PORT: 32450
```

**Advantages:**
- ✅ Lowest latency
- ✅ Simplest networking
- ✅ Direct port binding
- ✅ No service mesh overhead

**Considerations:**
- ⚠️ Port conflicts if multiple pods
- ⚠️ Requires DaemonSet pattern
- ⚠️ Security risk
- ⚠️ Less Kubernetes-native

**When to Use:**
- Migration from docker-compose
- Maximum performance requirements
- Single pod per node

---

### Option 4: Global Load Balancer with Cloud Armor

**Architecture:**
```
Internet (Anycast)
    ↓
Google Cloud Global Load Balancer
    ↓
Cloud Armor (DDoS protection, WAF)
    ↓
Backend Services (GKE clusters in multiple regions)
    ↓
Pods
```

**Note:** HTTP(S) Load Balancer doesn't support raw TCP well. Use TCP Proxy Load Balancer for global reach.

**Setup:**
```bash
# 1. Reserve global static IP
gcloud compute addresses create edgelake-global-ip \
  --global \
  --ip-version=IPV4

# 2. Create health check
gcloud compute health-checks create tcp edgelake-health-check \
  --port=32449 \
  --check-interval=10s

# 3. Create backend service
gcloud compute backend-services create edgelake-backend \
  --global \
  --protocol=TCP \
  --health-checks=edgelake-health-check \
  --timeout=3600s

# 4. Add GKE instance groups as backends
gcloud compute backend-services add-backend edgelake-backend \
  --global \
  --instance-group=gke-edgelake-cluster-xxxx \
  --instance-group-zone=us-central1-a

# 5. Create TCP proxy
gcloud compute target-tcp-proxies create edgelake-tcp-proxy \
  --backend-service=edgelake-backend

# 6. Create forwarding rule
gcloud compute forwarding-rules create edgelake-forwarding-rule \
  --global \
  --target-tcp-proxy=edgelake-tcp-proxy \
  --address=edgelake-global-ip \
  --ports=32448

# Repeat for ports 32449, 32450
```

**Helm Configuration:**
```yaml
node_configs:
  networking:
    OVERLAY_IP: "34.107.200.123"  # Global static IP
    ANYLOG_SERVER_PORT: 32448
```

**Advantages:**
- ✅ **Global Anycast IP** (low latency worldwide)
- ✅ Multi-region load balancing
- ✅ Cloud Armor DDoS protection
- ✅ Automatic failover
- ✅ Single static IP

**Considerations:**
- 💰💰 Higher cost: ~$18/month + ~$0.015/GB global forwarding
- 📝 Complex setup with multiple forwarding rules
- 🌍 Overkill for single-region

**Cost Estimate:**
- Forwarding rules: $0.025/hour × 3 ports × 730 = ~$54.75/month
- Data processing: $0.012-0.015/GB
- Total: ~$60-80/month

**When to Use:**
- Multi-region EdgeLake deployments
- Global EdgeLake network
- DDoS protection requirements

---

### Option 5: Private Service Connect (VPC-to-VPC)

**Architecture:**
```
EdgeLake Master VPC (vpc-111)
    ↓
Private Service Connect Endpoint
    ↓
Service Attachment (Internal LB)
    ↓
GKE Operator VPC (vpc-222)
    ↓
Internal LB → Pods
```

**Use Case:** Multi-VPC EdgeLake deployment with private connectivity.

**Setup:**
```bash
# 1. Create internal load balancer in operator VPC
# Helm values:
metadata:
  service_type: LoadBalancer
  annotations:
    cloud.google.com/load-balancer-type: "Internal"
    networking.gke.io/load-balancer-type: "Internal"

# 2. Get internal LB forwarding rule
LB_FR=$(gcloud compute forwarding-rules list \
  --filter="loadBalancingScheme:INTERNAL" \
  --format="get(name)")

# 3. Create service attachment
gcloud compute service-attachments create edgelake-attachment \
  --region=us-central1 \
  --producer-forwarding-rule=$LB_FR \
  --connection-preference=ACCEPT_AUTOMATIC \
  --nat-subnets=psc-subnet

# 4. In master VPC, create Private Service Connect endpoint
gcloud compute addresses create edgelake-psc-address \
  --region=us-central1 \
  --subnet=master-subnet

gcloud compute forwarding-rules create edgelake-psc-endpoint \
  --region=us-central1 \
  --network=master-vpc \
  --address=edgelake-psc-address \
  --target-service-attachment=edgelake-attachment

# 5. Configure EdgeLake
OVERLAY_IP: "10.1.2.3"  # PSC endpoint IP
```

**Advantages:**
- ✅ Private connectivity (no internet)
- ✅ Cross-VPC without peering
- ✅ Secure by design
- ✅ Consumer-side access control

**Considerations:**
- 💰 PSC endpoint: $0.01/hour + data transfer
- 📝 Complex setup
- 🔒 Only works within GCP
- ⚠️ Regional service

**When to Use:**
- Multi-VPC deployments
- Security requirement for private connectivity
- Cross-project EdgeLake network

---

### Option 6: Anthos Service Mesh (Istio)

**Architecture:**
```
Internet
    ↓
Istio Ingress Gateway (LoadBalancer)
    ↓
Virtual Service (TCP routing)
    ↓
Pods (with Envoy sidecar)
```

**Configuration:**
```yaml
# Gateway for TCP traffic
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: edgelake-gateway
spec:
  selector:
    istio: ingressgateway
  servers:
  - port:
      number: 32448
      name: tcp-edgelake
      protocol: TCP
    hosts:
    - "*"
  - port:
      number: 32449
      name: tcp-rest
      protocol: TCP
    hosts:
    - "*"

---
# VirtualService for routing
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: edgelake-operator-vs
spec:
  hosts:
  - "*"
  gateways:
  - edgelake-gateway
  tcp:
  - match:
    - port: 32448
    route:
    - destination:
        host: edgelake-operator-service
        port:
          number: 32448
  - match:
    - port: 32449
    route:
    - destination:
        host: edgelake-operator-service
        port:
          number: 32449
```

**Advantages:**
- ✅ Advanced traffic management
- ✅ Observability (metrics, tracing)
- ✅ mTLS between services
- ✅ Fine-grained access control

**Considerations:**
- 📝 Complex setup (requires Anthos/Istio)
- 💰 Additional control plane cost
- ⚠️ Sidecar overhead (CPU/memory)
- 🔧 Learning curve

**When to Use:**
- Already using Anthos Service Mesh
- Need advanced traffic management
- Security/compliance requirements for mTLS

---

## Comparison Matrix

| Option | Cost/Month | Setup | HA | Static IP | Latency | K8s Native | Best For |
|--------|------------|-------|-----|-----------|---------|------------|----------|
| **TCP/UDP LB** | $40-50 | Easy | ✅ | ✅ | Low | ✅ | **Production** |
| **NodePort + IP** | $7 | Medium | ❌ | ✅ | Lowest | ⚠️ | Dev/Test |
| **HostNetwork** | $7 | Easy | ❌ | ❌ | Lowest | ❌ | Simple |
| **Global LB** | $60-80 | Complex | ✅ | ✅ | Lowest | ✅ | Multi-region |
| **Private Service Connect** | $15-25 | Complex | ✅ | ❌ | Low | ✅ | Multi-VPC |
| **Anthos Mesh** | $50-70 | Complex | ✅ | ✅ | Medium | ✅ | Service mesh |

## Recommended Approach for GKE

### **Production: TCP/UDP Load Balancer with Reserved IP**

```yaml
# gcp-production-values.yaml
metadata:
  namespace: edgelake
  hostname: edgelake-operator-gcp
  app_name: edgelake-operator-gcp
  service_name: edgelake-operator-gcp-service
  service_type: LoadBalancer
  annotations:
    cloud.google.com/load-balancer-type: "External"
    # Reserve IP first: gcloud compute addresses create edgelake-operator-ip --region=us-central1
    cloud.google.com/load-balancer-ip: "34.123.45.67"

image:
  repository: gcr.io/<project-id>/edgelake-mcp
  tag: "amd64-latest"
  pull_policy: IfNotPresent
  # GKE has default access to GCR in same project, no secret needed

persistence:
  enabled: true
  storageClassName: "standard-rwo"  # GCE Persistent Disk
  accessMode: ReadWriteOnce
  anylog:
    size: 20Gi
  blockchain:
    size: 5Gi
  data:
    size: 50Gi
  scripts:
    size: 5Gi

resources:
  limits:
    cpu: "4000m"
    memory: "8Gi"
  requests:
    cpu: "1000m"
    memory: "2Gi"

node_configs:
  general:
    NODE_TYPE: operator
    NODE_NAME: ""
    COMPANY_NAME: "Your Company"
    DISABLE_CLI: false
    REMOTE_CLI: true

  networking:
    OVERLAY_IP: "34.123.45.67"  # Reserved static IP
    ANYLOG_SERVER_PORT: 32448
    ANYLOG_REST_PORT: 32449
    ANYLOG_BROKER_PORT: 32450

  blockchain:
    LEDGER_CONN: "master-lb-ip:32048"
    BLOCKCHAIN_SOURCE: master

  operator:
    CLUSTER_NAME: gcp-production-cluster
    DEFAULT_DBMS: production_db
```

### Deployment Steps

```bash
# 1. Set project and region
gcloud config set project <project-id>
gcloud config set compute/region us-central1

# 2. Create GKE cluster
gcloud container clusters create edgelake-cluster \
  --region=us-central1 \
  --num-nodes=1 \
  --machine-type=n1-standard-4 \
  --disk-size=100 \
  --disk-type=pd-standard \
  --enable-autorepair \
  --enable-autoupgrade \
  --enable-ip-alias \
  --network=default \
  --subnetwork=default

# 3. Get credentials
gcloud container clusters get-credentials edgelake-cluster \
  --region=us-central1

# 4. Create namespace
kubectl create namespace edgelake

# 5. Reserve static IP
gcloud compute addresses create edgelake-operator-ip \
  --region=us-central1

# Get the IP address
RESERVED_IP=$(gcloud compute addresses describe edgelake-operator-ip \
  --region=us-central1 \
  --format="get(address)")
echo "Reserved IP: $RESERVED_IP"

# 6. Update Helm values with reserved IP
# Edit gcp-production-values.yaml

# 7. Deploy EdgeLake
helm install edgelake-operator ./edgelake-operator \
  -f gcp-production-values.yaml \
  -n edgelake

# 8. Wait for load balancer provisioning (2-3 minutes)
kubectl get svc -n edgelake -w

# 9. Verify IP assignment
kubectl get svc edgelake-operator-gcp-service -n edgelake \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}'

# 10. Create firewall rule (if not automatically created)
gcloud compute firewall-rules create allow-edgelake-tcp \
  --allow=tcp:32448,tcp:32449,tcp:32450 \
  --source-ranges=0.0.0.0/0 \
  --description="EdgeLake operator ports"

# 11. Test connectivity
curl http://$RESERVED_IP:32449/get/status

# 12. Register with EdgeLake master
# Master will see: 34.123.45.67:32448 | operator | edgelake-operator-gcp
```

## Image Registry Options

### Option A: Google Container Registry (GCR) - **RECOMMENDED**

**Setup:**
```bash
# Enable Container Registry API
gcloud services enable containerregistry.googleapis.com

# Configure Docker for GCR
gcloud auth configure-docker

# Tag and push image
docker tag <hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest \
  gcr.io/<project-id>/edgelake-mcp:amd64-latest

docker push gcr.io/<project-id>/edgelake-mcp:amd64-latest
```

**Helm values:**
```yaml
image:
  repository: gcr.io/<project-id>/edgelake-mcp
  tag: amd64-latest
  # No secret needed - GKE has default access to GCR in same project
```

**Cost:** $0.026/GB/month storage

### Option B: Artifact Registry (Newer, Recommended for New Projects)

**Setup:**
```bash
# Enable Artifact Registry API
gcloud services enable artifactregistry.googleapis.com

# Create repository
gcloud artifacts repositories create edgelake \
  --repository-format=docker \
  --location=us-central1 \
  --description="EdgeLake container images"

# Configure Docker
gcloud auth configure-docker us-central1-docker.pkg.dev

# Tag and push
docker tag <hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest \
  us-central1-docker.pkg.dev/<project-id>/edgelake/edgelake-mcp:amd64-latest

docker push us-central1-docker.pkg.dev/<project-id>/edgelake/edgelake-mcp:amd64-latest
```

**Helm values:**
```yaml
image:
  repository: us-central1-docker.pkg.dev/<project-id>/edgelake/edgelake-mcp
  tag: amd64-latest
```

**Cost:** $0.10/GB/month storage (first 0.5GB free)

### Option C: Keep Private Registry (VPN/Interconnect)

If your private registry is accessible from GCP:

```yaml
image:
  repository: <hub-tailscale-ip>:5000/edgelake-mcp
  tag: amd64-latest
```

**Requires:**
- Cloud VPN or Cloud Interconnect to on-prem
- Firewall rules allowing GKE → registry:5000

## Storage Considerations

### Persistent Disk Types

```yaml
persistence:
  storageClassName: "standard-rwo"  # Recommended for most workloads
  # Options:
  # - standard-rwo: Standard persistent disk (HDD-backed)
  # - premium-rwo: SSD persistent disk
  # - balanced-rwo: Balanced SSD (between standard and premium)
```

**Cost Comparison (us-central1):**
- Standard (HDD): $0.04/GB/month
- Balanced SSD: $0.10/GB/month
- Performance SSD (premium): $0.17/GB/month

**For 75Gi total (20+5+50):**
- Standard: ~$3/month
- Balanced SSD: ~$7.50/month
- Premium SSD: ~$12.75/month

### Filestore (NFS) - Alternative

For shared storage across pods:

```yaml
persistence:
  storageClassName: "filestore-sc"
```

**Use Case:** Multiple operator replicas sharing data

**Cost:**
- Basic HDD: $0.20/GB/month (1TB minimum)
- Basic SSD: $0.30/GB/month (2.5TB minimum)

**Not recommended** unless you need shared file storage.

### Regional Persistent Disks

For HA storage that replicates across zones:

```yaml
persistence:
  storageClassName: "regional-pd"  # Custom StorageClass
```

**Setup:**
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: regional-pd
provisioner: kubernetes.io/gce-pd
parameters:
  type: pd-standard
  replication-type: regional-pd
  zones: us-central1-a,us-central1-b
```

**Cost:** 2× standard disk price (~$0.08/GB/month)

## Security Considerations

### Network Security

**Firewall Rules:**
```bash
# Allow EdgeLake TCP port from other operators
gcloud compute firewall-rules create allow-edgelake-from-master \
  --allow=tcp:32448 \
  --source-ranges=34.100.1.2/32 \
  --target-tags=gke-edgelake-cluster-node \
  --description="Allow EdgeLake TCP from master"

# Allow REST API (public or restricted)
gcloud compute firewall-rules create allow-edgelake-rest \
  --allow=tcp:32449 \
  --source-ranges=0.0.0.0/0 \
  --target-tags=gke-edgelake-cluster-node

# Or restrict to specific IPs
gcloud compute firewall-rules create allow-edgelake-rest-office \
  --allow=tcp:32449 \
  --source-ranges=203.0.113.0/24 \
  --target-tags=gke-edgelake-cluster-node
```

**Network Policies:**
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: edgelake-operator-policy
  namespace: edgelake
spec:
  podSelector:
    matchLabels:
      app: edgelake-operator-gcp
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - ipBlock:
        cidr: 0.0.0.0/0  # Or restrict to known IPs
    ports:
    - protocol: TCP
      port: 32448
    - protocol: TCP
      port: 32449
  egress:
  - to:
    - ipBlock:
        cidr: 0.0.0.0/0
    ports:
    - protocol: TCP
      port: 32048  # Master node
```

### Pod Security

```yaml
# In deployment.yaml
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: edgelake-operator
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: false
          capabilities:
            drop:
              - ALL
```

### Workload Identity (IAM)

If EdgeLake needs GCP API access:

```bash
# 1. Create service account
gcloud iam service-accounts create edgelake-operator-sa \
  --display-name="EdgeLake Operator Service Account"

# 2. Grant permissions (example: GCS access)
gcloud projects add-iam-policy-binding <project-id> \
  --member="serviceAccount:edgelake-operator-sa@<project-id>.iam.gserviceaccount.com" \
  --role="roles/storage.objectViewer"

# 3. Bind K8s SA to GCP SA
gcloud iam service-accounts add-iam-policy-binding \
  edgelake-operator-sa@<project-id>.iam.gserviceaccount.com \
  --role="roles/iam.workloadIdentityUser" \
  --member="serviceAccount:<project-id>.svc.id.goog[edgelake/edgelake-operator-sa]"

# 4. Create K8s service account with annotation
kubectl create serviceaccount edgelake-operator-sa -n edgelake

kubectl annotate serviceaccount edgelake-operator-sa \
  -n edgelake \
  iam.gke.io/gcp-service-account=edgelake-operator-sa@<project-id>.iam.gserviceaccount.com

# 5. Use in Helm
serviceAccount:
  create: false
  name: edgelake-operator-sa
```

### Binary Authorization

Enforce only signed images can run:

```bash
# Enable Binary Authorization
gcloud services enable binaryauthorization.googleapis.com

# Create attestor
gcloud container binauthz attestors create edgelake-attestor \
  --attestation-authority-note=edgelake-note \
  --attestation-authority-note-project=<project-id>

# Create policy
cat > binauth-policy.yaml <<EOF
admissionWhitelistPatterns:
- namePattern: gcr.io/<project-id>/edgelake-mcp:*
defaultAdmissionRule:
  requireAttestationsBy:
  - projects/<project-id>/attestors/edgelake-attestor
  evaluationMode: REQUIRE_ATTESTATION
  enforcementMode: ENFORCED_BLOCK_AND_AUDIT_LOG
globalPolicyEvaluationMode: ENABLE
EOF

gcloud container binauthz policy import binauth-policy.yaml
```

## Monitoring & Observability

### Google Cloud Operations (formerly Stackdriver)

**Automatically enabled** on GKE with Cloud Logging and Cloud Monitoring.

**View logs:**
```bash
# Via gcloud
gcloud logging read "resource.type=k8s_container AND resource.labels.namespace_name=edgelake" \
  --limit=50 \
  --format=json

# Via kubectl
kubectl logs -n edgelake -l app=edgelake-operator-gcp --tail=100
```

**Metrics Dashboard:**
- Go to Cloud Console → Kubernetes Engine → Workloads
- Select `edgelake-operator-gcp` deployment
- View CPU, memory, network metrics

**Custom Metrics:**
```yaml
# If EdgeLake exposes Prometheus metrics
apiVersion: v1
kind: Service
metadata:
  name: edgelake-metrics
  annotations:
    cloud.google.com/backend-config: '{"ports": {"32449":"edgelake-backendconfig"}}'
spec:
  type: ClusterIP
  ports:
  - name: metrics
    port: 32449
    targetPort: 32449
```

### Prometheus + Grafana (Alternative)

```bash
# Install Prometheus Operator
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus-community/kube-prometheus-stack \
  -n monitoring --create-namespace

# Create ServiceMonitor
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: edgelake-operator
  namespace: edgelake
spec:
  selector:
    matchLabels:
      app: edgelake-operator-gcp
  endpoints:
  - port: rest-api
    path: /metrics
```

### Cloud Trace (Distributed Tracing)

If EdgeLake supports OpenTelemetry:

```yaml
# Install OpenTelemetry Collector
helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
helm install otel-collector open-telemetry/opentelemetry-collector \
  --set mode=daemonset \
  --set config.exporters.googlecloud.project=<project-id>
```

## Cost Estimate: Production Deployment

| Component | Monthly Cost |
|-----------|--------------|
| GKE Cluster (Autopilot mode) | $72 |
| 3× n1-standard-4 nodes (Standard mode) | $273 (3 × $91) |
| TCP/UDP Load Balancer | $18 |
| Forwarding rules (3 ports) | $22 |
| Persistent Disks (Standard, 75Gi) | $3 |
| Static IP (in use) | $0 |
| Data Transfer (estimate 100GB egress) | $12 |
| GCR Storage (5GB) | $0.13 |
| **Total (Standard mode)** | **~$328/month** |
| **Total (Autopilot mode)** | **~$127/month** |

**Cost Optimization:**
- Use GKE Autopilot: Pay per pod resource usage (~60% savings)
- Use Spot VMs for nodes: Save 60-91% on compute
- Use Standard (HDD) persistent disks: $0.04/GB vs $0.10/GB SSD
- Regional cluster only if HA required (adds control plane cost)

### Autopilot vs Standard Comparison

**GKE Autopilot (Recommended for simplicity):**
- ✅ No node management
- ✅ Pay per pod resource usage
- ✅ Auto-scaling, auto-repair, auto-upgrade
- ⚠️ Less control over node configuration
- 💰 ~$127/month for EdgeLake operator

**GKE Standard (More control):**
- ✅ Full control over nodes, networking
- ✅ Can use Spot/Preemptible VMs
- ⚠️ Requires node management
- 💰 ~$328/month (can reduce to ~$100 with Spot VMs)

## Migration Path from On-Prem

### Phase 1: Hybrid (Keep existing + Add GCP)

1. Deploy EdgeLake operator on GKE
2. Configure to connect to existing master (<hub-tailscale-ip>:32048)
3. Run in parallel with on-prem operators
4. Gradually shift workload to GCP

### Phase 2: Full GCP Migration

1. Deploy master node on GKE
2. Migrate all operators to GKE
3. Update blockchain configuration
4. Decommission on-prem infrastructure

### Phase 3: Multi-Region

1. Deploy in multiple GCP regions (us-central1, europe-west1, asia-east1)
2. Use Global Load Balancer for static IPs
3. Implement region-aware routing
4. Cloud CDN for content delivery

## Troubleshooting

### Load Balancer Health Checks Failing

```bash
# Check pod status
kubectl get pods -n edgelake -l app=edgelake-operator-gcp

# Check service
kubectl describe svc edgelake-operator-gcp-service -n edgelake

# Test from node
NODE_NAME=$(kubectl get pods -n edgelake -l app=edgelake-operator-gcp -o jsonpath='{.items[0].spec.nodeName}')
NODE_IP=$(kubectl get node $NODE_NAME -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}')

# SSH to node (requires enabling SSH)
gcloud compute ssh $NODE_NAME --zone=us-central1-a

# Test from node
curl http://localhost:32449/get/status

# Check firewall rules
gcloud compute firewall-rules list --filter="name:k8s"
```

### Pod Can't Pull Image from GCR

```bash
# Check if GCR is enabled
gcloud services list --enabled | grep containerregistry

# Check image exists
gcloud container images list --repository=gcr.io/<project-id>

# Check pod events
kubectl describe pod <pod-name> -n edgelake

# Verify GKE service account has GCR access
gcloud projects get-iam-policy <project-id> \
  --flatten="bindings[].members" \
  --filter="bindings.members:*gke*"
```

### Can't Connect to EdgeLake TCP Port

```bash
# Check load balancer status
kubectl get svc edgelake-operator-gcp-service -n edgelake

# Check firewall rules
gcloud compute firewall-rules list --filter="allowed.ports:32448"

# Create rule if missing
gcloud compute firewall-rules create allow-edgelake-tcp \
  --allow=tcp:32448,tcp:32449,tcp:32450 \
  --source-ranges=0.0.0.0/0

# Test from outside GCP
nc -zv <load-balancer-ip> 32448
telnet <load-balancer-ip> 32448

# Check backend health
gcloud compute backend-services get-health <backend-service-name> \
  --region=us-central1
```

### Persistent Disk Mounting Issues

```bash
# Check PVC status
kubectl get pvc -n edgelake

# Check PV status
kubectl get pv

# Describe PVC for events
kubectl describe pvc edgelake-operator-gcp-anylog-pvc -n edgelake

# Check node zones match disk zones
kubectl get nodes --show-labels | grep topology.kubernetes.io/zone

# Recreate PVC if in wrong zone
kubectl delete pvc <pvc-name> -n edgelake
# Helm will recreate it
```

## Advanced Configurations

### Multi-Zonal Deployment

```yaml
# Spread pods across zones for HA
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchLabels:
            app: edgelake-operator-gcp
        topologyKey: topology.kubernetes.io/zone
```

### Autoscaling (if EdgeLake supports multiple replicas)

```yaml
# HorizontalPodAutoscaler
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: edgelake-operator-hpa
  namespace: edgelake
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: edgelake-operator-gcp-deployment
  minReplicas: 1
  maxReplicas: 5
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

### Backup Strategy

```bash
# Install Velero for backup/restore
helm repo add vmware-tanzu https://vmware-tanzu.github.io/helm-charts
helm install velero vmware-tanzu/velero \
  --namespace velero --create-namespace \
  --set-file credentials.secretContents.cloud=./gcp-credentials.json \
  --set configuration.provider=gcp \
  --set configuration.backupStorageLocation.bucket=edgelake-backups \
  --set configuration.backupStorageLocation.config.project=<project-id>

# Create backup schedule
velero schedule create edgelake-daily \
  --schedule="0 2 * * *" \
  --include-namespaces edgelake
```

## Conclusion

**For production GKE deployment:**

✅ **Use TCP/UDP Load Balancer with Reserved Static IP**
- Kubernetes-native, HA, manageable cost
- Static IP for stable EdgeLake network configuration
- Health checks and automatic failover
- Easy integration with existing EdgeLake network

**Alternative approaches:**
- NodePort + Reserved IP: Dev/test only
- HostNetwork: Simple migrations from docker-compose
- Global Load Balancer: Multi-region deployments
- Private Service Connect: Multi-VPC secure connectivity
- GKE Autopilot: Serverless Kubernetes (easiest management)

**GKE Autopilot is recommended** for new deployments due to simplicity and cost-effectiveness. Use Standard mode if you need specific node configurations or want to use Spot VMs for cost savings.

The recommended approach balances cost, complexity, and production-readiness while maintaining compatibility with EdgeLake's networking requirements.
