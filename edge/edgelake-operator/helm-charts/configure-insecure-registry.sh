#!/bin/bash
# Configure k3s to allow insecure registry access
# Usage: ./configure-insecure-registry.sh

set -e

# Configuration
REMOTE_HOST="100.74.102.38"
REMOTE_USER="<user>"
REGISTRY="<hub-tailscale-ip>:5000"

echo "======================================"
echo "Configure Insecure Registry for k3s"
echo "Target: ${REMOTE_HOST}"
echo "Registry: ${REGISTRY}"
echo "======================================"
echo

# Check SSH connectivity
echo "Step 1: Checking SSH connectivity..."
if ! ssh -o ConnectTimeout=5 "${REMOTE_USER}@${REMOTE_HOST}" "echo 'Connected'" &>/dev/null; then
    echo "ERROR: Cannot connect to ${REMOTE_HOST}"
    exit 1
fi
echo "✓ SSH connection successful"
echo

# Configure containerd for insecure registry
echo "Step 2: Configuring containerd for insecure registry..."
ssh "${REMOTE_USER}@${REMOTE_HOST}" bash << EOF
set -e

# Create registries.yaml for k3s
echo "  - Creating /etc/rancher/k3s/registries.yaml..."
sudo tee /etc/rancher/k3s/registries.yaml > /dev/null << 'YAML'
mirrors:
  "${REGISTRY}":
    endpoint:
      - "http://${REGISTRY}"
configs:
  "${REGISTRY}":
    tls:
      insecure_skip_verify: true
YAML

echo "✓ Registry configuration created"
EOF
echo

# Restart k3s to apply changes
echo "Step 3: Restarting k3s service..."
ssh "${REMOTE_USER}@${REMOTE_HOST}" bash << 'EOF'
set -e

echo "  - Restarting k3s..."
sudo systemctl restart k3s

echo "  - Waiting for k3s to be ready..."
sleep 10

# Wait for k3s to be responsive
for i in {1..30}; do
    if KUBECONFIG=~/.kube/config kubectl get nodes &>/dev/null; then
        echo "✓ k3s is ready"
        exit 0
    fi
    echo "    Waiting for k3s... ($i/30)"
    sleep 2
done

echo "⚠ k3s may still be starting up"
EOF
echo

# Verify configuration
echo "Step 4: Verifying configuration..."
ssh "${REMOTE_USER}@${REMOTE_HOST}" bash << EOF
set -e

echo "  - Checking registries.yaml..."
if [ -f /etc/rancher/k3s/registries.yaml ]; then
    echo "✓ Configuration file exists"
    cat /etc/rancher/k3s/registries.yaml
else
    echo "✗ Configuration file not found"
    exit 1
fi
EOF
echo

# Test image pull
echo "Step 5: Testing image pull from registry..."
ssh "${REMOTE_USER}@${REMOTE_HOST}" bash << EOF
set -e

export PATH=\$HOME/bin:\$PATH
export KUBECONFIG=~/.kube/config

echo "  - Creating test pod to pull image..."
kubectl run test-registry-pull \\
    --image=${REGISTRY}/edgelake-mcp:amd64-latest \\
    --restart=Never \\
    --command -- sleep 10 2>/dev/null || echo "    (pod may already exist)"

echo "  - Waiting for image pull..."
sleep 5

# Check pod status
POD_STATUS=\$(kubectl get pod test-registry-pull -o jsonpath='{.status.phase}' 2>/dev/null || echo "NotFound")
echo "  - Pod status: \$POD_STATUS"

# Check events
echo "  - Recent events:"
kubectl describe pod test-registry-pull | grep -A 5 "Events:" | tail -5

# Cleanup
echo "  - Cleaning up test pod..."
kubectl delete pod test-registry-pull --ignore-not-found

if [ "\$POD_STATUS" == "Running" ] || kubectl get events | grep -q "Successfully pulled"; then
    echo "✓ Image pull successful"
else
    echo "⚠ Image pull may have issues - check events above"
fi
EOF
echo

echo "======================================"
echo "Configuration Complete!"
echo "======================================"
echo
echo "k3s has been configured to use insecure registry: ${REGISTRY}"
echo
echo "Next steps:"
echo "  1. Run pre-deployment checks: ./pre-deploy-checks.sh"
echo "  2. Deploy EdgeLake: ./deploy-test.sh"
echo
