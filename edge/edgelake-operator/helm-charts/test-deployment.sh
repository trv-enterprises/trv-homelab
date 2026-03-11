#!/bin/bash
# Test EdgeLake operator deployment on k3s
# Usage: ./test-deployment.sh

set -e

# Configuration
REMOTE_HOST="100.74.102.38"
REMOTE_USER="<user>"
RELEASE_NAME="edgelake-k8s-test"
NAMESPACE="default"
MASTER_HOST="<hub-tailscale-ip>"
MASTER_PORT="32048"
OPERATOR_PORT="32449"

echo "======================================"
echo "EdgeLake Deployment Tests"
echo "Target: ${REMOTE_HOST}"
echo "======================================"
echo

# Test 1: Pod Status
echo "Test 1: Checking Pod Status"
echo "------------------------------"
POD_STATUS=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "KUBECONFIG=~/.kube/config kubectl get pods -l app=edgelake-operator-k8s -n ${NAMESPACE} -o jsonpath='{.items[0].status.phase}'" 2>/dev/null || echo "NotFound")

if [ "$POD_STATUS" == "Running" ]; then
    echo "✓ Pod is running"
else
    echo "✗ Pod status: $POD_STATUS"
    echo "  Checking pod events..."
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "KUBECONFIG=~/.kube/config kubectl describe pod -l app=edgelake-operator-k8s -n ${NAMESPACE} | tail -20"
    exit 1
fi
echo

# Test 2: Service Endpoints
echo "Test 2: Checking Service Endpoints"
echo "------------------------------"
NODEPORT=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "export PATH=\$HOME/bin:\$PATH; KUBECONFIG=~/.kube/config kubectl get svc -l app=edgelake-operator-k8s -n ${NAMESPACE} -o jsonpath='{.items[0].spec.ports[?(@.name==\"rest-api\")].nodePort}'" 2>/dev/null || echo "NotFound")

if [ "$NODEPORT" != "NotFound" ] && [ ! -z "$NODEPORT" ]; then
    echo "✓ Service NodePort: $NODEPORT"
else
    echo "✗ Service NodePort not found"
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "KUBECONFIG=~/.kube/config kubectl get svc -l app=edgelake-operator-k8s -n ${NAMESPACE}"
    exit 1
fi
echo

# Test 3: REST API Connectivity (from local)
echo "Test 3: Testing REST API Connectivity"
echo "------------------------------"
echo "  Checking http://${REMOTE_HOST}:${OPERATOR_PORT}/ ..."
if curl -s -m 5 "http://${REMOTE_HOST}:${OPERATOR_PORT}/" > /dev/null 2>&1; then
    echo "✓ REST API is accessible from local machine"

    # Get status
    echo "  Getting node status..."
    STATUS=$(curl -s -m 5 "http://${REMOTE_HOST}:${OPERATOR_PORT}/get/status" 2>/dev/null || echo "error")
    if [ "$STATUS" != "error" ]; then
        echo "  Status response: ${STATUS:0:100}..."
    fi
else
    echo "✗ REST API not accessible (this may be normal if EdgeLake is still initializing)"
    echo "  Hint: Check pod logs for startup progress"
fi
echo

# Test 4: Persistent Volumes
echo "Test 4: Checking Persistent Volumes"
echo "------------------------------"
PVC_COUNT=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "KUBECONFIG=~/.kube/config kubectl get pvc -l app=edgelake-operator-k8s -n ${NAMESPACE} --no-headers | wc -l" 2>/dev/null || echo "0")

if [ "$PVC_COUNT" -ge 4 ]; then
    echo "✓ Found $PVC_COUNT PVCs"
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "KUBECONFIG=~/.kube/config kubectl get pvc -l app=edgelake-operator-k8s -n ${NAMESPACE}"
else
    echo "⚠ Expected 4 PVCs, found $PVC_COUNT"
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "KUBECONFIG=~/.kube/config kubectl get pvc -l app=edgelake-operator-k8s -n ${NAMESPACE}"
fi
echo

# Test 5: Pod Logs Check
echo "Test 5: Checking Pod Logs for Errors"
echo "------------------------------"
echo "  Fetching last 50 lines..."
LOGS=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "KUBECONFIG=~/.kube/config kubectl logs -l app=edgelake-operator-k8s -n ${NAMESPACE} --tail=50" 2>/dev/null || echo "error")

if [ "$LOGS" == "error" ]; then
    echo "✗ Could not fetch logs"
else
    ERROR_COUNT=$(echo "$LOGS" | grep -i "error\|failed\|exception" | grep -v "0 errors" | wc -l || echo "0")
    if [ "$ERROR_COUNT" -eq 0 ]; then
        echo "✓ No errors found in recent logs"
    else
        echo "⚠ Found $ERROR_COUNT potential errors in logs:"
        echo "$LOGS" | grep -i "error\|failed\|exception" | grep -v "0 errors" | head -5
    fi
fi
echo

# Test 6: Network Connectivity to Master
echo "Test 6: Testing Connectivity to Master Node"
echo "------------------------------"
echo "  Testing from pod to master at ${MASTER_HOST}:${MASTER_PORT}..."
MASTER_TEST=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "KUBECONFIG=~/.kube/config kubectl exec -n ${NAMESPACE} \$(KUBECONFIG=~/.kube/config kubectl get pod -l app=edgelake-operator-k8s -n ${NAMESPACE} -o jsonpath='{.items[0].metadata.name}') -- timeout 5 nc -zv ${MASTER_HOST} ${MASTER_PORT}" 2>&1 || echo "failed")

if echo "$MASTER_TEST" | grep -q "succeeded\|open"; then
    echo "✓ Can reach master node from pod"
else
    echo "✗ Cannot reach master node from pod"
    echo "  Response: $MASTER_TEST"
fi
echo

# Summary
echo "======================================"
echo "Test Summary"
echo "======================================"
echo
echo "✓ Passed tests indicate the deployment is working"
echo "⚠ Warnings may require investigation"
echo "✗ Failed tests need to be addressed"
echo
echo "Useful commands:"
echo "  View logs:      ssh ${REMOTE_USER}@${REMOTE_HOST} 'KUBECONFIG=~/.kube/config kubectl logs -f -l app=edgelake-operator-k8s -n ${NAMESPACE}'"
echo "  Shell access:   ssh ${REMOTE_USER}@${REMOTE_HOST} 'KUBECONFIG=~/.kube/config kubectl exec -it \$(KUBECONFIG=~/.kube/config kubectl get pod -l app=edgelake-operator-k8s -n ${NAMESPACE} -o jsonpath=\"{.items[0].metadata.name}\") -- /bin/bash'"
echo "  Pod details:    ssh ${REMOTE_USER}@${REMOTE_HOST} 'KUBECONFIG=~/.kube/config kubectl describe pod -l app=edgelake-operator-k8s -n ${NAMESPACE}'"
echo "  Delete release: ./cleanup-test.sh"
echo
