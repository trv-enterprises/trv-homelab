#!/bin/bash
# Pre-deployment checks for EdgeLake on k3s
# Usage: ./pre-deploy-checks.sh

# Removed set -e to avoid issues with check failures

# Configuration
REMOTE_HOST="100.74.102.38"
REMOTE_USER="<user>"
MASTER_HOST="<hub-tailscale-ip>"
MASTER_PORT="32048"
OPERATOR_PORTS="32448 32449 32450"
# Use user kubeconfig and PATH
KUBECTL="export PATH=\$HOME/bin:\$PATH; KUBECONFIG=~/.kube/config kubectl"
HELM="export PATH=\$HOME/bin:\$PATH; KUBECONFIG=~/.kube/config helm"

echo "======================================"
echo "Pre-Deployment Checks"
echo "Target: ${REMOTE_HOST}"
echo "======================================"
echo

CHECKS_PASSED=0
CHECKS_FAILED=0
CHECKS_WARNING=0

# Helper functions
check_pass() {
    echo "✓ $1"
    ((CHECKS_PASSED++))
}

check_fail() {
    echo "✗ $1"
    ((CHECKS_FAILED++))
}

check_warn() {
    echo "⚠ $1"
    ((CHECKS_WARNING++))
}

# Check 1: SSH Connectivity
echo "Check 1: SSH Connectivity"
echo "------------------------------"
if ssh -o ConnectTimeout=5 "${REMOTE_USER}@${REMOTE_HOST}" "echo 'OK'" &>/dev/null; then
    check_pass "Can connect to ${REMOTE_HOST} via SSH"
else
    check_fail "Cannot connect to ${REMOTE_HOST} via SSH"
fi
echo

# Check 2: Kubernetes cluster
echo "Check 2: Kubernetes Cluster"
echo "------------------------------"
K8S_VERSION=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "${KUBECTL} version 2>/dev/null | grep 'Server Version' | head -1" || echo "error")
if [ "$K8S_VERSION" != "error" ]; then
    check_pass "Kubernetes is running: ${K8S_VERSION}"
else
    check_fail "Kubernetes is not accessible"
fi

# Check node status
NODE_STATUS=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "${KUBECTL} get nodes --no-headers 2>/dev/null | awk '{print \$2}'" || echo "error")
if [ "$NODE_STATUS" == "Ready" ]; then
    check_pass "Kubernetes node is Ready"
else
    check_warn "Kubernetes node status: ${NODE_STATUS}"
fi
echo

# Check 3: Helm
echo "Check 3: Helm"
echo "------------------------------"
HELM_VERSION=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "${HELM} version --short 2>/dev/null" || echo "error")
if [ "$HELM_VERSION" != "error" ]; then
    check_pass "Helm is installed: ${HELM_VERSION}"
else
    check_fail "Helm is not installed or not accessible"
fi
echo

# Check 4: Storage class
echo "Check 4: Storage Class"
echo "------------------------------"
STORAGE_CLASS=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "${KUBECTL} get storageclass --no-headers 2>/dev/null | grep '(default)' | awk '{print \$1}'" || echo "error")
if [ "$STORAGE_CLASS" != "error" ] && [ ! -z "$STORAGE_CLASS" ]; then
    check_pass "Default storage class found: ${STORAGE_CLASS}"
else
    check_warn "No default storage class (will need to specify in values)"
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "${KUBECTL} get storageclass 2>/dev/null || echo 'No storage classes found'"
fi
echo

# Check 5: Available resources
echo "Check 5: Available Resources"
echo "------------------------------"
RESOURCES=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "${KUBECTL} top nodes 2>/dev/null" || echo "error")
if [ "$RESOURCES" != "error" ]; then
    echo "$RESOURCES"
    check_pass "Resource metrics available"
else
    check_warn "Cannot get resource metrics (metrics-server may not be installed)"
fi
echo

# Check 6: Port availability
echo "Check 6: Port Availability"
echo "------------------------------"
for PORT in $OPERATOR_PORTS; do
    PORT_CHECK=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "nc -zv 127.0.0.1 ${PORT} 2>&1" || echo "available")
    if echo "$PORT_CHECK" | grep -q "succeeded\|open"; then
        check_warn "Port ${PORT} is already in use on ${REMOTE_HOST}"
    else
        check_pass "Port ${PORT} is available"
    fi
done
echo

# Check 7: Network connectivity to master
echo "Check 7: Master Node Connectivity"
echo "------------------------------"
echo "  Testing connection to master at ${MASTER_HOST}:${MASTER_PORT}..."
if nc -zv -w 5 "${MASTER_HOST}" "${MASTER_PORT}" 2>&1 | grep -q "succeeded\|open"; then
    check_pass "Can reach master node from local machine"
else
    check_warn "Cannot reach master node from local machine (may work from k8s pod)"
fi

# Test from remote host
REMOTE_NC=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "timeout 5 nc -zv ${MASTER_HOST} ${MASTER_PORT}" 2>&1 || echo "failed")
if echo "$REMOTE_NC" | grep -q "succeeded\|open"; then
    check_pass "Can reach master node from ${REMOTE_HOST}"
else
    check_fail "Cannot reach master node from ${REMOTE_HOST}"
    echo "  This is required for blockchain sync!"
fi
echo

# Check 8: Tailscale connectivity
echo "Check 8: Tailscale Overlay Network"
echo "------------------------------"
TAILSCALE_STATUS=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "tailscale status 2>/dev/null | grep ${REMOTE_HOST}" || echo "error")
if [ "$TAILSCALE_STATUS" != "error" ]; then
    check_pass "Tailscale is active on ${REMOTE_HOST}"
else
    check_warn "Cannot verify Tailscale status (may not be installed on host)"
fi
echo

# Check 9: Docker image accessibility
echo "Check 9: Docker Image"
echo "------------------------------"
IMAGE_CHECK=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "${KUBECTL} run test-image-pull --image=anylogco/edgelake-network:1.3.2500 --dry-run=server 2>&1" || echo "error")
if echo "$IMAGE_CHECK" | grep -q "created\|would be created"; then
    check_pass "Docker image anylogco/edgelake-network:1.3.2500 is accessible"
else
    check_warn "Could not verify image accessibility"
fi
ssh "${REMOTE_USER}@${REMOTE_HOST}" "${KUBECTL} delete pod test-image-pull --ignore-not-found" &>/dev/null || true
echo

# Check 10: Existing deployments
echo "Check 10: Existing Deployments"
echo "------------------------------"
EXISTING=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "${HELM} list -n default | grep edgelake" || echo "")
if [ -z "$EXISTING" ]; then
    check_pass "No conflicting EdgeLake deployments found"
else
    check_warn "Found existing EdgeLake deployment(s):"
    echo "$EXISTING"
fi
echo

# Summary
echo "======================================"
echo "Pre-Deployment Check Summary"
echo "======================================"
echo "Passed:   ${CHECKS_PASSED}"
echo "Warnings: ${CHECKS_WARNING}"
echo "Failed:   ${CHECKS_FAILED}"
echo

if [ $CHECKS_FAILED -gt 0 ]; then
    echo "⚠ There are failed checks that should be addressed before deployment."
    echo "  Review the failures above and resolve issues."
    exit 1
elif [ $CHECKS_WARNING -gt 0 ]; then
    echo "⚠ There are warnings that may need attention."
    echo "  Review the warnings above. You may proceed with deployment."
    exit 0
else
    echo "✓ All checks passed! Ready to deploy."
    echo
    echo "Next step: ./deploy-test.sh"
    exit 0
fi
