# EdgeLake Helm Chart Testing Guide

This guide provides scripts and procedures for testing EdgeLake deployment on k3s cluster (trv-srv-011).

## Quick Start

```bash
cd /path/to/trv-edgelake-infra/helm-charts

# 1. Run pre-deployment checks
./pre-deploy-checks.sh

# 2. Deploy to k3s
./deploy-test.sh

# 3. Test the deployment
./test-deployment.sh

# 4. Cleanup when done
./cleanup-test.sh
```

## Test Environment

**Target Host:** trv-srv-011 (100.74.102.38)
- Kubernetes: k3s cluster
- Network: Tailscale overlay (100.74.102.38)
- Ports: 32448 (TCP), 32449 (REST), 32450 (MQTT)

**Master Node:** trv-srv-001 (<hub-tailscale-ip>:32048)

**Configuration:** `test-values-trv-srv-011.yaml`

## Scripts

### 1. pre-deploy-checks.sh

Validates environment before deployment:
- SSH connectivity
- Kubernetes cluster health
- Helm availability
- Storage class configuration
- Port availability
- Network connectivity to master
- Tailscale status
- Docker image accessibility
- Existing deployments

**Usage:**
```bash
./pre-deploy-checks.sh
```

**Exit codes:**
- 0: All checks passed or warnings only
- 1: Critical failures found

### 2. deploy-test.sh

Deploys EdgeLake operator to k3s cluster:
1. Runs pre-deployment checks
2. Syncs Helm chart and values to remote host
3. Installs/upgrades Helm release
4. Waits for pod readiness
5. Displays deployment information

**Usage:**
```bash
./deploy-test.sh
```

**Features:**
- Automatic upgrade detection
- Interactive confirmation for upgrades
- Comprehensive error checking
- Real-time status updates

### 3. test-deployment.sh

Validates the running deployment:
- Pod status and health
- Service endpoints and NodePorts
- REST API connectivity
- Persistent volume status
- Log analysis for errors
- Network connectivity to master

**Usage:**
```bash
./test-deployment.sh
```

**Output:**
- ✓ Test passed
- ⚠ Warning (needs investigation)
- ✗ Test failed (needs fix)

### 4. cleanup-test.sh

Removes test deployment completely:
1. Shows current deployment state
2. Deletes Helm release
3. Waits for pod termination
4. Removes persistent volumes (DATA LOSS!)
5. Cleans remote files
6. Verifies cleanup

**Usage:**
```bash
# Interactive mode (confirms before deletion)
./cleanup-test.sh

# Force mode (no confirmation)
./cleanup-test.sh --force
```

**⚠️ WARNING:** This deletes ALL data in persistent volumes!

## Test Workflow

### Initial Deployment

```bash
# Check environment
./pre-deploy-checks.sh

# Deploy
./deploy-test.sh

# Verify
./test-deployment.sh

# View logs
ssh <user>@100.74.102.38 'kubectl logs -f -l app=edgelake-k8s-test'
```

### Accessing the Deployment

**View logs:**
```bash
ssh <user>@100.74.102.38 'kubectl logs -f -l app=edgelake-k8s-test -n default'
```

**Shell access:**
```bash
ssh <user>@100.74.102.38 'kubectl exec -it $(kubectl get pod -l app=edgelake-k8s-test -n default -o jsonpath="{.items[0].metadata.name}") -- /bin/bash'
```

**EdgeLake CLI access:**
```bash
ssh <user>@100.74.102.38 'kubectl exec -it $(kubectl get pod -l app=edgelake-k8s-test -n default -o jsonpath="{.items[0].metadata.name}") -- edgelake-cli'
```

**Pod details:**
```bash
ssh <user>@100.74.102.38 'kubectl describe pod -l app=edgelake-k8s-test -n default'
```

**Test REST API:**
```bash
# From local machine (via Tailscale)
curl http://100.74.102.38:32449/
curl http://100.74.102.38:32449/get/status
```

### Making Changes

**Update configuration:**
1. Edit `test-values-trv-srv-011.yaml`
2. Run `./deploy-test.sh` (will upgrade existing release)
3. Run `./test-deployment.sh` to verify

**Update Helm chart:**
1. Modify files in `edgelake-operator/`
2. Run `./deploy-test.sh` to sync and upgrade

### Troubleshooting

**Pod not starting:**
```bash
ssh <user>@100.74.102.38 'kubectl describe pod -l app=edgelake-k8s-test'
ssh <user>@100.74.102.38 'kubectl logs -l app=edgelake-k8s-test --previous'
```

**PVC issues:**
```bash
ssh <user>@100.74.102.38 'kubectl get pvc -l app=edgelake-k8s-test'
ssh <user>@100.74.102.38 'kubectl describe pvc -l app=edgelake-k8s-test'
```

**Network issues:**
```bash
# Test from pod to master
ssh <user>@100.74.102.38 'kubectl exec $(kubectl get pod -l app=edgelake-k8s-test -o jsonpath="{.items[0].metadata.name}") -- nc -zv <hub-tailscale-ip> 32048'

# Test NodePort accessibility
nc -zv 100.74.102.38 32449
```

**Service issues:**
```bash
ssh <user>@100.74.102.38 'kubectl get svc -l app=edgelake-k8s-test -o wide'
ssh <user>@100.74.102.38 'kubectl get endpoints -l app=edgelake-k8s-test'
```

### Complete Cleanup

```bash
# Remove everything
./cleanup-test.sh

# Verify removal
ssh <user>@100.74.102.38 'kubectl get all -l app=edgelake-k8s-test'
```

## Common Issues

### Issue: Pod stuck in Pending
**Cause:** PVC not bound, insufficient resources, or node selector mismatch
**Fix:**
```bash
ssh <user>@100.74.102.38 'kubectl describe pod -l app=edgelake-k8s-test'
# Check events section for details
```

### Issue: Cannot reach master node
**Cause:** Firewall, Tailscale routing, or master not running
**Fix:**
```bash
# From k8s host
ssh <user>@100.74.102.38 'nc -zv <hub-tailscale-ip> 32048'

# From pod
ssh <user>@100.74.102.38 'kubectl exec $(kubectl get pod -l app=edgelake-k8s-test -o jsonpath="{.items[0].metadata.name}") -- nc -zv <hub-tailscale-ip> 32048'
```

### Issue: Port already in use
**Cause:** Another service using 32448/32449/32450
**Fix:** Change ports in `test-values-trv-srv-011.yaml` and redeploy

### Issue: Image pull errors
**Cause:** Network issues or image not available
**Fix:**
```bash
ssh <user>@100.74.102.38 'kubectl describe pod -l app=edgelake-k8s-test | grep -A 5 Events'
```

## Next Steps After Testing

Once testing is successful:

1. **Document configuration:** Update deployment docs with tested values
2. **Create production values:** Copy test values, adjust for production
3. **Open Horizon integration:** Package Helm deployment for OH orchestration
4. **Ingress setup:** If needed, add ingress controller and rules
5. **Monitoring:** Add Prometheus/Grafana for metrics
6. **Backup strategy:** Configure PVC backup/restore procedures

## Files

- `test-values-trv-srv-011.yaml` - Test configuration values
- `pre-deploy-checks.sh` - Pre-deployment validation
- `deploy-test.sh` - Deploy to k3s
- `test-deployment.sh` - Validate deployment
- `cleanup-test.sh` - Remove deployment
- `edgelake-operator/` - Helm chart directory

## References

- EdgeLake Documentation: `/path/to/documentation`
- Helm Chart: `helm-charts/edgelake-operator/`
- Project Context: `../CLAUDE.md`
- OH Integration Plan: `../OH_DEPLOYMENT_PLAN.md`
