#!/bin/bash
# Deploy EdgeLake operator to trv-srv-011 k3s cluster for testing
# Usage: ./deploy-test.sh

set -e

# Configuration
REMOTE_HOST="100.74.102.38"
REMOTE_USER="<user>"
REMOTE_PATH="/home/<user>/helm-test"
RELEASE_NAME="edgelake-k8s-test"
NAMESPACE="default"
VALUES_FILE="test-values-trv-srv-011.yaml"
CHART_DIR="edgelake-operator"
# Use user kubeconfig and PATH
KUBECTL="export PATH=\$HOME/bin:\$PATH; KUBECONFIG=~/.kube/config kubectl"
HELM="export PATH=\$HOME/bin:\$PATH; KUBECONFIG=~/.kube/config helm"

echo "======================================"
echo "EdgeLake Helm Deployment Test"
echo "Target: ${REMOTE_HOST}"
echo "Release: ${RELEASE_NAME}"
echo "======================================"
echo

# Step 1: Pre-deployment checks
echo "Step 1: Running pre-deployment checks..."
echo "  - Checking local files..."
if [ ! -f "${VALUES_FILE}" ]; then
    echo "ERROR: Values file not found: ${VALUES_FILE}"
    exit 1
fi

if [ ! -d "${CHART_DIR}" ]; then
    echo "ERROR: Chart directory not found: ${CHART_DIR}"
    exit 1
fi

echo "  - Checking remote host connectivity..."
if ! ssh -o ConnectTimeout=5 "${REMOTE_USER}@${REMOTE_HOST}" "echo 'Connected'" &>/dev/null; then
    echo "ERROR: Cannot connect to ${REMOTE_HOST}"
    exit 1
fi

echo "  - Checking k3s availability..."
if ! ssh "${REMOTE_USER}@${REMOTE_HOST}" "${KUBECTL} version --client &>/dev/null"; then
    echo "ERROR: kubectl not available on remote host"
    exit 1
fi

echo "  - Checking if release already exists..."
if ssh "${REMOTE_USER}@${REMOTE_HOST}" "${HELM} list -n ${NAMESPACE} | grep -q ${RELEASE_NAME}" 2>/dev/null; then
    echo "WARNING: Release ${RELEASE_NAME} already exists!"
    read -p "Do you want to upgrade it? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Deployment cancelled."
        exit 0
    fi
    DEPLOYMENT_TYPE="upgrade"
else
    DEPLOYMENT_TYPE="install"
fi

echo "✓ Pre-deployment checks passed"
echo

# Step 2: Sync files to remote host
echo "Step 2: Syncing files to remote host..."
echo "  - Creating remote directory..."
ssh "${REMOTE_USER}@${REMOTE_HOST}" "mkdir -p ${REMOTE_PATH}"

echo "  - Syncing Helm chart..."
rsync -avz --delete "${CHART_DIR}/" "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_PATH}/${CHART_DIR}/"

echo "  - Syncing values file..."
scp "${VALUES_FILE}" "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_PATH}/"

echo "✓ Files synced"
echo

# Step 3: Deploy Helm chart
echo "Step 3: Deploying Helm chart..."
if [ "${DEPLOYMENT_TYPE}" == "upgrade" ]; then
    echo "  - Upgrading existing release..."
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "cd ${REMOTE_PATH} && ${HELM} upgrade ${RELEASE_NAME} ./${CHART_DIR} -f ${VALUES_FILE} -n ${NAMESPACE}"
else
    echo "  - Installing new release..."
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "cd ${REMOTE_PATH} && ${HELM} install ${RELEASE_NAME} ./${CHART_DIR} -f ${VALUES_FILE} -n ${NAMESPACE}"
fi

echo "✓ Helm chart deployed"
echo

# Step 4: Wait for deployment
echo "Step 4: Waiting for deployment to be ready..."
echo "  - Waiting up to 5 minutes for pod to be ready..."
if ssh "${REMOTE_USER}@${REMOTE_HOST}" "${KUBECTL} wait --for=condition=ready pod -l app=${RELEASE_NAME} -n ${NAMESPACE} --timeout=300s" 2>/dev/null; then
    echo "✓ Pod is ready"
else
    echo "WARNING: Pod may not be ready yet. Check status manually."
fi
echo

# Step 5: Display deployment info
echo "Step 5: Deployment Information"
echo "======================================"
echo
ssh "${REMOTE_USER}@${REMOTE_HOST}" << EOF
echo "Pods:"
${KUBECTL} get pods -l app=edgelake-k8s-test -n default -o wide
echo
echo "Services:"
${KUBECTL} get svc -l app=edgelake-k8s-test -n default
echo
echo "PVCs:"
${KUBECTL} get pvc -l app=edgelake-k8s-test -n default
echo
echo "Helm Release:"
${HELM} list -n default | grep edgelake-k8s-test
EOF

echo
echo "======================================"
echo "Deployment Complete!"
echo "======================================"
echo
echo "Next steps:"
echo "  1. View logs: ssh ${REMOTE_USER}@${REMOTE_HOST} '${KUBECTL} logs -f -l app=${RELEASE_NAME} -n ${NAMESPACE}'"
echo "  2. Run tests: ./test-deployment.sh"
echo "  3. Cleanup: ./cleanup-test.sh"
echo
