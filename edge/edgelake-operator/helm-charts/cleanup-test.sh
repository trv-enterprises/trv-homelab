#!/bin/bash
# Cleanup EdgeLake test deployment on k3s
# Usage: ./cleanup-test.sh [--force]

set -e

# Configuration
REMOTE_HOST="100.74.102.38"
REMOTE_USER="<user>"
REMOTE_PATH="/home/<user>/helm-test"
RELEASE_NAME="edgelake-k8s-test"
NAMESPACE="default"

# Check for force flag
FORCE=false
if [ "$1" == "--force" ]; then
    FORCE=true
fi

echo "======================================"
echo "EdgeLake Test Cleanup"
echo "Target: ${REMOTE_HOST}"
echo "Release: ${RELEASE_NAME}"
echo "======================================"
echo

# Confirm deletion unless --force
if [ "$FORCE" == false ]; then
    echo "WARNING: This will delete:"
    echo "  - Helm release: ${RELEASE_NAME}"
    echo "  - All pods and services"
    echo "  - All persistent volumes and data"
    echo "  - Remote files in ${REMOTE_PATH}"
    echo
    read -p "Are you sure you want to continue? (yes/no) " -r
    echo
    if [[ ! $REPLY =~ ^[Yy][Ee][Ss]$ ]]; then
        echo "Cleanup cancelled."
        exit 0
    fi
fi

# Step 1: Show current state
echo "Step 1: Current deployment state"
echo "------------------------------"
ssh "${REMOTE_USER}@${REMOTE_HOST}" << EOF
echo "Helm release:"
KUBECONFIG=~/.kube/config helm list -n ${NAMESPACE} | grep ${RELEASE_NAME} || echo "Not found"
echo
echo "Pods:"
KUBECONFIG=~/.kube/config kubectl get pods -l app=${RELEASE_NAME} -n ${NAMESPACE} || echo "Not found"
echo
echo "PVCs:"
KUBECONFIG=~/.kube/config kubectl get pvc -l app=${RELEASE_NAME} -n ${NAMESPACE} || echo "Not found"
EOF
echo

# Step 2: Delete Helm release
echo "Step 2: Deleting Helm release"
echo "------------------------------"
if ssh "${REMOTE_USER}@${REMOTE_HOST}" "KUBECONFIG=~/.kube/config helm list -n ${NAMESPACE} | grep -q ${RELEASE_NAME}" 2>/dev/null; then
    echo "  Uninstalling ${RELEASE_NAME}..."
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "KUBECONFIG=~/.kube/config helm uninstall ${RELEASE_NAME} -n ${NAMESPACE}"
    echo "✓ Helm release deleted"
else
    echo "  Release not found, skipping..."
fi
echo

# Step 3: Wait for pod deletion
echo "Step 3: Waiting for resources to terminate"
echo "------------------------------"
echo "  Waiting up to 60 seconds..."
for i in {1..12}; do
    POD_COUNT=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "KUBECONFIG=~/.kube/config kubectl get pods -l app=${RELEASE_NAME} -n ${NAMESPACE} --no-headers 2>/dev/null | wc -l" || echo "0")
    if [ "$POD_COUNT" -eq 0 ]; then
        echo "✓ All pods terminated"
        break
    fi
    echo "  Waiting... ($i/12) - $POD_COUNT pod(s) still terminating"
    sleep 5
done
echo

# Step 4: Delete PVCs (if any remain)
echo "Step 4: Cleaning up persistent volumes"
echo "------------------------------"
PVC_LIST=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "KUBECONFIG=~/.kube/config kubectl get pvc -l app=${RELEASE_NAME} -n ${NAMESPACE} -o jsonpath='{.items[*].metadata.name}' 2>/dev/null" || echo "")

if [ ! -z "$PVC_LIST" ]; then
    echo "  Found PVCs to delete: $PVC_LIST"
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "KUBECONFIG=~/.kube/config kubectl delete pvc -l app=${RELEASE_NAME} -n ${NAMESPACE}"
    echo "✓ PVCs deleted"
else
    echo "  No PVCs found, skipping..."
fi
echo

# Step 5: Clean up remote files
echo "Step 5: Cleaning up remote files"
echo "------------------------------"
if ssh "${REMOTE_USER}@${REMOTE_HOST}" "[ -d ${REMOTE_PATH} ]"; then
    echo "  Deleting ${REMOTE_PATH}..."
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "rm -rf ${REMOTE_PATH}"
    echo "✓ Remote files deleted"
else
    echo "  Directory not found, skipping..."
fi
echo

# Step 6: Verify cleanup
echo "Step 6: Verifying cleanup"
echo "------------------------------"
ssh "${REMOTE_USER}@${REMOTE_HOST}" << EOF
echo "Remaining resources with label app=${RELEASE_NAME}:"
echo
echo "Pods:"
KUBECONFIG=~/.kube/config kubectl get pods -l app=${RELEASE_NAME} -n ${NAMESPACE} 2>/dev/null || echo "  None"
echo
echo "Services:"
KUBECONFIG=~/.kube/config kubectl get svc -l app=${RELEASE_NAME} -n ${NAMESPACE} 2>/dev/null || echo "  None"
echo
echo "PVCs:"
KUBECONFIG=~/.kube/config kubectl get pvc -l app=${RELEASE_NAME} -n ${NAMESPACE} 2>/dev/null || echo "  None"
echo
echo "PVs (checking for orphaned volumes):"
KUBECONFIG=~/.kube/config kubectl get pv | grep ${RELEASE_NAME} || echo "  None"
EOF

echo
echo "======================================"
echo "Cleanup Complete!"
echo "======================================"
echo
echo "All test resources have been removed."
echo "You can now run ./deploy-test.sh to deploy again."
echo
