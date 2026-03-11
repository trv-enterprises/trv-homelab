# EdgeLake Operator Deployment on AWS EKS

This document analyzes deployment strategies for EdgeLake operator nodes on AWS Elastic Kubernetes Service (EKS).

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

## AWS EKS Networking Options

### Option 1: Network Load Balancer (NLB) - **RECOMMENDED**

**Architecture:**
```
Internet
    ↓
AWS Network Load Balancer (Layer 4)
    ↓ (Target: NodePort or IP mode)
EKS Worker Nodes
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
    # Use Network Load Balancer (not Classic LB)
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"

    # Internet-facing or internal
    service.beta.kubernetes.io/aws-load-balancer-scheme: "internet-facing"

    # Optional: Use existing Elastic IPs for static addresses
    service.beta.kubernetes.io/aws-load-balancer-eip-allocations: "eipalloc-xxxxx,eipalloc-yyyyy"

    # Health check configuration
    service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol: "TCP"
    service.beta.kubernetes.io/aws-load-balancer-healthcheck-port: "32449"

    # Connection settings
    service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout: "3600"

node_configs:
  networking:
    # NLB DNS name - set after deployment
    OVERLAY_IP: "edgelake-operator-xxxxx.elb.us-east-1.amazonaws.com"
    ANYLOG_SERVER_PORT: 32448
    ANYLOG_REST_PORT: 32449
    ANYLOG_BROKER_PORT: 32450
```

**Post-Deployment Setup:**
```bash
# 1. Deploy with Helm
helm install edgelake-operator ./edgelake-operator -f aws-values.yaml

# 2. Get NLB address
NLB_DNS=$(kubectl get svc edgelake-operator-service -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')
echo "NLB Address: $NLB_DNS"

# 3. Update EdgeLake configuration with NLB address
# Either:
#   a) Upgrade Helm release with OVERLAY_IP set
#   b) Use ConfigMap hot-reload if supported
#   c) Restart pod to pick up new config

# 4. Verify connectivity
curl http://$NLB_DNS:32449/get/status
```

**Advantages:**
- ✅ Fully managed AWS service
- ✅ High availability across AZs
- ✅ Layer 4 TCP load balancing (perfect for EdgeLake)
- ✅ Health checks ensure traffic to healthy pods only
- ✅ Static IP option via Elastic IP allocation
- ✅ Kubernetes-native (standard LoadBalancer service)
- ✅ Auto-scales with pod replicas

**Considerations:**
- 💰 Cost: ~$16-18/month per NLB + data processing ($0.006/GB)
- 📝 DNS name changes if service recreated (use EIP allocation for static IP)
- 🔒 Security group configuration required for pod-to-NLB communication

**Cost Estimate:**
- NLB: $0.0225/hour × 730 hours = ~$16.43/month
- Data processing: $0.006/GB processed
- Total: ~$20-30/month depending on traffic

---

### Option 2: NodePort + Elastic IP

**Architecture:**
```
Internet
    ↓
Elastic IP (attached to worker node)
    ↓
EKS Worker Node (ports 32448-32450)
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
    # Elastic IP of worker node
    OVERLAY_IP: "52.1.2.3"
    ANYLOG_SERVER_PORT: 32448
    ANYLOG_REST_PORT: 32449
    ANYLOG_BROKER_PORT: 32450
```

**Manual Setup Required:**
```bash
# 1. Allocate Elastic IP
aws ec2 allocate-address --domain vpc
# Output: eipalloc-xxxxx, PublicIp: 52.1.2.3

# 2. Get worker node instance ID
NODE_INSTANCE=$(kubectl get nodes -o jsonpath='{.items[0].spec.providerID}' | cut -d/ -f5)

# 3. Associate EIP with node
aws ec2 associate-address \
  --instance-id $NODE_INSTANCE \
  --allocation-id eipalloc-xxxxx

# 4. Configure security group
aws ec2 authorize-security-group-ingress \
  --group-id sg-xxxxx \
  --protocol tcp \
  --port 32448 \
  --cidr 0.0.0.0/0  # Or restrict to EdgeLake network

aws ec2 authorize-security-group-ingress \
  --group-id sg-xxxxx \
  --protocol tcp \
  --port 32449 \
  --cidr 0.0.0.0/0

aws ec2 authorize-security-group-ingress \
  --group-id sg-xxxxx \
  --protocol tcp \
  --port 32450 \
  --cidr 0.0.0.0/0

# 5. Deploy EdgeLake with OVERLAY_IP set to Elastic IP
helm install edgelake-operator ./edgelake-operator \
  --set node_configs.networking.OVERLAY_IP="52.1.2.3" \
  -f aws-nodeport-values.yaml
```

**Advantages:**
- ✅ Lower cost (only EIP: $3.60/month if used)
- ✅ Static IP address
- ✅ Simple architecture
- ✅ Direct node access (no LB hop)

**Considerations:**
- ⚠️ Manual EIP management
- ⚠️ Single point of failure (one node)
- ⚠️ Pod affinity required to ensure pod stays on node with EIP
- ⚠️ Node replacement breaks connectivity
- ⚠️ Security group management
- ⚠️ Not HA - if node fails, operator is unreachable

**When to Use:**
- Development/testing environments
- Cost-sensitive deployments
- Single-node clusters
- Controlled environments with known node IPs

---

### Option 3: HostNetwork Mode

**Architecture:**
```
Internet
    ↓
Elastic IP (attached to worker node)
    ↓
EKS Worker Node - Pod in host network namespace
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
          hostPort: 32448  # Explicitly bind to host
        - containerPort: 32449
          hostPort: 32449
        - containerPort: 32450
          hostPort: 32450
```

**Values:**
```yaml
hostNetwork: true  # Custom value to enable hostNetwork

node_configs:
  networking:
    OVERLAY_IP: "52.1.2.3"  # Worker node Elastic IP
    ANYLOG_SERVER_PORT: 32448
    ANYLOG_REST_PORT: 32449
    ANYLOG_BROKER_PORT: 32450
```

**Advantages:**
- ✅ Lowest latency (no service/proxy overhead)
- ✅ Simplest networking (like docker-compose)
- ✅ Direct port binding to host
- ✅ No service mesh required

**Considerations:**
- ⚠️ Port conflicts if multiple pods on same node
- ⚠️ Requires DaemonSet or pod anti-affinity
- ⚠️ Security risk (pod has full host network access)
- ⚠️ Less Kubernetes-native
- ⚠️ Breaks pod-to-pod service discovery

**When to Use:**
- Migration from docker-compose
- Maximum performance requirements
- Single pod per node deployments
- Controlled network environments

---

### Option 4: AWS Global Accelerator + NLB

**Architecture:**
```
Internet
    ↓
AWS Global Accelerator (Anycast IPs: 75.2.60.5, 99.83.190.7)
    ↓
Network Load Balancer(s) in multiple regions
    ↓
EKS Clusters
    ↓
Pods
```

**Setup:**
```bash
# 1. Create NLB (via LoadBalancer service)
helm install edgelake-operator ./edgelake-operator -f aws-nlb-values.yaml

# 2. Get NLB ARN
NLB_ARN=$(aws elbv2 describe-load-balancers \
  --names edgelake-operator-nlb \
  --query 'LoadBalancers[0].LoadBalancerArn' \
  --output text)

# 3. Create Global Accelerator
aws globalaccelerator create-accelerator \
  --name edgelake-operator-accelerator \
  --ip-address-type IPV4 \
  --enabled

# 4. Create listener
aws globalaccelerator create-listener \
  --accelerator-arn <accelerator-arn> \
  --port-ranges FromPort=32448,ToPort=32450 \
  --protocol TCP

# 5. Create endpoint group
aws globalaccelerator create-endpoint-group \
  --listener-arn <listener-arn> \
  --endpoint-configurations EndpointId=$NLB_ARN \
  --endpoint-group-region us-east-1

# 6. Get static IPs
aws globalaccelerator describe-accelerator \
  --accelerator-arn <accelerator-arn> \
  --query 'Accelerator.IpSets[0].IpAddresses'
# Output: ["75.2.60.5", "99.83.190.7"]
```

**Helm Configuration:**
```yaml
node_configs:
  networking:
    OVERLAY_IP: "75.2.60.5"  # Global Accelerator static IP
    ANYLOG_SERVER_PORT: 32448
```

**Advantages:**
- ✅ **Static Anycast IP addresses** (never change)
- ✅ Global edge network for low latency
- ✅ Automatic failover between regions
- ✅ DDoS protection at edge
- ✅ Multi-region HA

**Considerations:**
- 💰💰 Higher cost: $0.025/hour + $0.015/hour per endpoint + data transfer
- 📝 More complex setup
- 🌍 Overkill for single-region deployment

**Cost Estimate:**
- Accelerator: $0.025/hour × 730 = $18.25/month
- Endpoint: $0.015/hour × 730 = $10.95/month
- Data transfer: $0.015/GB
- Total: ~$30-50/month + data transfer

**When to Use:**
- Multi-region EdgeLake deployments
- Requirement for static IP addresses
- Global EdgeLake network
- Mission-critical availability

---

### Option 5: AWS PrivateLink (VPC-to-VPC)

**Architecture:**
```
EdgeLake Master VPC (vpc-111111)
    ↓
VPC Endpoint (vpce-xxxxx)
    ↓
Endpoint Service (backed by NLB)
    ↓
EKS Operator VPC (vpc-222222)
    ↓
NLB → Pods
```

**Use Case:** Multi-VPC EdgeLake deployment where master and operators are in different VPCs/accounts.

**Setup:**
```bash
# 1. Create internal NLB in operator VPC
# Helm values:
metadata:
  service_type: LoadBalancer
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    service.beta.kubernetes.io/aws-load-balancer-internal: "true"

# 2. Create VPC Endpoint Service
aws ec2 create-vpc-endpoint-service-configuration \
  --network-load-balancer-arns $NLB_ARN \
  --acceptance-required

# 3. In master VPC, create VPC Endpoint
aws ec2 create-vpc-endpoint \
  --vpc-id vpc-111111 \
  --service-name com.amazonaws.vpce.us-east-1.vpce-svc-xxxxx \
  --vpc-endpoint-type Interface \
  --subnet-ids subnet-xxxxx

# 4. Configure EdgeLake
OVERLAY_IP: "vpce-xxxxx.vpce-svc-xxxxx.us-east-1.vpce.amazonaws.com"
```

**Advantages:**
- ✅ Private connectivity (no internet)
- ✅ Cross-account/cross-VPC without peering
- ✅ Secure by design
- ✅ Scalable

**Considerations:**
- 💰 VPC Endpoint: $0.01/hour + data transfer
- 📝 Complex networking setup
- 🔒 Only works within AWS
- ⚠️ DNS resolution required

**When to Use:**
- Multi-account AWS deployments
- Security requirement for private connectivity
- No internet egress allowed
- Cross-VPC EdgeLake network

---

## Comparison Matrix

| Option | Cost/Month | Setup | HA | Static IP | Latency | K8s Native | Best For |
|--------|------------|-------|-----|-----------|---------|------------|----------|
| **NLB** | $20-30 | Easy | ✅ | Optional | Low | ✅ | **Production** |
| **NodePort + EIP** | $3-5 | Medium | ❌ | ✅ | Lowest | ⚠️ | Dev/Test |
| **HostNetwork** | $3-5 | Easy | ❌ | ❌ | Lowest | ❌ | Simple |
| **Global Accelerator** | $30-50 | Complex | ✅ | ✅ | Lowest | ✅ | Multi-region |
| **PrivateLink** | $15-25 | Complex | ✅ | ❌ | Low | ✅ | Multi-VPC |

## Recommended Approach for AWS

### **Production: NLB with Elastic IP Allocation**

```yaml
# aws-production-values.yaml
metadata:
  namespace: edgelake
  hostname: edgelake-operator-aws
  app_name: edgelake-operator-aws
  service_name: edgelake-operator-aws-service
  service_type: LoadBalancer
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    service.beta.kubernetes.io/aws-load-balancer-scheme: "internet-facing"
    # Allocate Elastic IPs first, then use allocation IDs
    service.beta.kubernetes.io/aws-load-balancer-eip-allocations: "eipalloc-xxxxx"
    service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol: "TCP"
    service.beta.kubernetes.io/aws-load-balancer-healthcheck-port: "32449"
    service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout: "3600"

image:
  repository: <your-ecr-registry>/edgelake-mcp
  tag: "amd64-latest"
  pull_policy: IfNotPresent
  secret_name: ecr-registry-secret  # For ECR authentication

persistence:
  enabled: true
  storageClassName: "gp3"  # AWS EBS gp3 volumes
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
    NODE_NAME: ""  # Auto-generated
    COMPANY_NAME: "Your Company"
    DISABLE_CLI: false
    REMOTE_CLI: true

  networking:
    OVERLAY_IP: "52.1.2.3"  # Set to Elastic IP after allocation
    ANYLOG_SERVER_PORT: 32448
    ANYLOG_REST_PORT: 32449
    ANYLOG_BROKER_PORT: 32450

  blockchain:
    LEDGER_CONN: "master-nlb-xxxxx.elb.us-east-1.amazonaws.com:32048"
    BLOCKCHAIN_SOURCE: master

  operator:
    CLUSTER_NAME: aws-production-cluster
    DEFAULT_DBMS: production_db
```

### Deployment Steps

```bash
# 1. Create EKS cluster
eksctl create cluster \
  --name edgelake-cluster \
  --region us-east-1 \
  --nodegroup-name standard-workers \
  --node-type t3.large \
  --nodes 3 \
  --nodes-min 2 \
  --nodes-max 5

# 2. Create namespace
kubectl create namespace edgelake

# 3. Setup ECR authentication (if using ECR)
aws ecr get-login-password --region us-east-1 | \
  kubectl create secret docker-registry ecr-registry-secret \
    --docker-server=<account>.dkr.ecr.us-east-1.amazonaws.com \
    --docker-username=AWS \
    --docker-password=$(aws ecr get-login-password) \
    -n edgelake

# 4. Allocate Elastic IP
aws ec2 allocate-address --domain vpc --region us-east-1
# Note the AllocationId: eipalloc-xxxxx

# 5. Update Helm values with EIP allocation ID
# Edit aws-production-values.yaml

# 6. Deploy EdgeLake
helm install edgelake-operator ./edgelake-operator \
  -f aws-production-values.yaml \
  -n edgelake

# 7. Wait for NLB provisioning (2-3 minutes)
kubectl get svc -n edgelake -w

# 8. Get NLB DNS and IP
kubectl get svc edgelake-operator-aws-service -n edgelake \
  -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'

# 9. Verify Elastic IP is attached
nslookup <nlb-dns-name>

# 10. Test connectivity
curl http://<elastic-ip>:32449/get/status

# 11. Register with EdgeLake master
# Master will see: 52.1.2.3:32448 | operator | edgelake-operator-aws
```

## Image Registry Options

### Option A: Amazon ECR (Elastic Container Registry)

**Setup:**
```bash
# Create repository
aws ecr create-repository \
  --repository-name edgelake-mcp \
  --region us-east-1

# Tag and push image
docker tag <hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest \
  <account>.dkr.ecr.us-east-1.amazonaws.com/edgelake-mcp:amd64-latest

aws ecr get-login-password --region us-east-1 | \
  docker login --username AWS --password-stdin \
  <account>.dkr.ecr.us-east-1.amazonaws.com

docker push <account>.dkr.ecr.us-east-1.amazonaws.com/edgelake-mcp:amd64-latest
```

**Helm values:**
```yaml
image:
  repository: <account>.dkr.ecr.us-east-1.amazonaws.com/edgelake-mcp
  tag: amd64-latest
  secret_name: ecr-registry-secret
```

**Cost:** $0.10/GB/month storage

### Option B: Keep Private Registry (VPN/VPC Peering)

If your private registry (<hub-tailscale-ip>:5000) is accessible from AWS:

```yaml
image:
  repository: <hub-tailscale-ip>:5000/edgelake-mcp
  tag: amd64-latest
  pull_policy: Always
```

**Requires:**
- VPN connection from AWS VPC to on-prem
- Or VPC peering/PrivateLink
- Security group allowing EKS → registry:5000

## Storage Considerations

### EBS Volume Types

```yaml
persistence:
  storageClassName: "gp3"  # Recommended
  # Options:
  # - gp3: General Purpose SSD (default, best price/performance)
  # - gp2: General Purpose SSD (legacy)
  # - io2: Provisioned IOPS SSD (high performance)
  # - st1: Throughput Optimized HDD (large data, sequential)
```

**Cost Comparison (us-east-1):**
- gp3: $0.08/GB/month + $0.005/IOPS (3000 IOPS free)
- gp2: $0.10/GB/month
- io2: $0.125/GB/month + $0.065/IOPS

**For 75Gi total (20+5+50):**
- gp3: ~$6/month
- gp2: ~$7.50/month

### EFS (Elastic File System) - Alternative

For shared storage across pods:

```yaml
persistence:
  storageClassName: "efs-sc"
```

**Use Case:** Multiple operator replicas sharing data (if EdgeLake supports it)

**Cost:** $0.30/GB/month (more expensive but shared)

## Security Considerations

### Network Security

**Security Group Rules:**
```bash
# Allow EdgeLake TCP port from other operators
aws ec2 authorize-security-group-ingress \
  --group-id sg-xxxxx \
  --protocol tcp \
  --port 32448 \
  --source-group sg-master  # Master node SG

# Allow REST API (if public)
aws ec2 authorize-security-group-ingress \
  --group-id sg-xxxxx \
  --protocol tcp \
  --port 32449 \
  --cidr 0.0.0.0/0

# Or restrict to specific IPs
aws ec2 authorize-security-group-ingress \
  --group-id sg-xxxxx \
  --protocol tcp \
  --port 32449 \
  --cidr 203.0.113.0/24  # Your office IP range
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
      containers:
      - name: edgelake-operator
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: false  # EdgeLake needs write access
          capabilities:
            drop:
              - ALL
```

### IAM Roles (IRSA)

If EdgeLake needs AWS API access:

```bash
# Create IAM role
eksctl create iamserviceaccount \
  --name edgelake-operator-sa \
  --namespace edgelake \
  --cluster edgelake-cluster \
  --attach-policy-arn arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess \
  --approve

# Use in Helm
serviceAccount:
  create: true
  name: edgelake-operator-sa
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::<account>:role/edgelake-operator-role
```

## Monitoring & Observability

### CloudWatch Container Insights

```bash
# Install CloudWatch agent
eksctl utils install-cloudwatch-insights \
  --cluster edgelake-cluster \
  --region us-east-1
```

**Metrics Available:**
- Pod CPU/Memory usage
- Network I/O
- Disk I/O
- Container logs

### Prometheus + Grafana (Alternative)

```bash
# Install Prometheus Operator
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus-community/kube-prometheus-stack \
  -n monitoring --create-namespace

# Add ServiceMonitor for EdgeLake
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: edgelake-operator
spec:
  selector:
    matchLabels:
      app: edgelake-operator-aws
  endpoints:
  - port: rest-api
    path: /metrics  # If EdgeLake exposes Prometheus metrics
```

## Cost Estimate: Production Deployment

| Component | Monthly Cost |
|-----------|--------------|
| EKS Control Plane | $73 |
| 3× t3.large worker nodes | $150 (3 × $50) |
| Network Load Balancer | $18 |
| EBS Storage (gp3, 75Gi) | $6 |
| Data Transfer (estimate 100GB/month) | $9 |
| ECR Storage (5GB) | $0.50 |
| **Total** | **~$256/month** |

**Cost Optimization:**
- Use Spot instances for workers: Save 70% (~$45 vs $150)
- Use Fargate for operators: Pay per pod (~$30-40/month per pod)
- Reserved instances: Save 30-40% on sustained workloads

## Migration Path from On-Prem

### Phase 1: Hybrid (Keep existing + Add AWS)

1. Deploy EdgeLake operator on AWS EKS
2. Configure to connect to existing master (<hub-tailscale-ip>:32048)
3. Run in parallel with on-prem operators
4. Gradually shift workload to AWS

### Phase 2: Full AWS Migration

1. Deploy master node on AWS
2. Migrate all operators to EKS
3. Update blockchain configuration
4. Decommission on-prem infrastructure

### Phase 3: Multi-Region

1. Deploy master in multiple regions
2. Deploy operators globally
3. Use Global Accelerator for static IPs
4. Implement region-aware routing

## Troubleshooting

### NLB Health Checks Failing

```bash
# Check pod status
kubectl get pods -n edgelake -l app=edgelake-operator-aws

# Check health check config
kubectl describe svc edgelake-operator-aws-service -n edgelake

# Test from worker node
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
curl http://$NODE_IP:32449/get/status

# Check security groups
aws ec2 describe-security-groups --group-ids sg-xxxxx
```

### Pod Can't Pull Image from ECR

```bash
# Refresh ECR secret
kubectl delete secret ecr-registry-secret -n edgelake

aws ecr get-login-password --region us-east-1 | \
  kubectl create secret docker-registry ecr-registry-secret \
    --docker-server=<account>.dkr.ecr.us-east-1.amazonaws.com \
    --docker-username=AWS \
    --docker-password=$(cat -) \
    -n edgelake

# Check pod events
kubectl describe pod <pod-name> -n edgelake
```

### Can't Connect to EdgeLake TCP Port

```bash
# Check NLB target health
aws elbv2 describe-target-health \
  --target-group-arn <target-group-arn>

# Check security group on worker nodes
aws ec2 describe-instances \
  --instance-ids <instance-id> \
  --query 'Reservations[0].Instances[0].SecurityGroups'

# Test from outside AWS
nc -zv <elastic-ip> 32448
telnet <elastic-ip> 32448
```

## Conclusion

**For production AWS EKS deployment:**

✅ **Use Network Load Balancer with Elastic IP allocation**
- Kubernetes-native, HA, manageable cost
- Static IP for stable EdgeLake network configuration
- Health checks and automatic failover
- Easy integration with existing EdgeLake network

**Alternative approaches:**
- NodePort + EIP: Dev/test environments only
- HostNetwork: Simple migrations from docker-compose
- Global Accelerator: Multi-region deployments
- PrivateLink: Multi-VPC secure connectivity

The recommended NLB approach balances cost, complexity, and production-readiness while maintaining compatibility with EdgeLake's networking requirements.
