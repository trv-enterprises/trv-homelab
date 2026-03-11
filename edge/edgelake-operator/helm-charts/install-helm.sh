#!/bin/bash
# Install Helm on remote k3s host
# Usage: ./install-helm.sh

set -e

# Configuration
REMOTE_HOST="100.74.102.38"
REMOTE_USER="<user>"
HELM_VERSION="v3.16.3"  # Latest stable as of Nov 2024

echo "======================================"
echo "Helm Installation Script"
echo "Target: ${REMOTE_HOST}"
echo "Version: ${HELM_VERSION}"
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

# Check if Helm is already installed
echo "Step 2: Checking if Helm is already installed..."
if ssh "${REMOTE_USER}@${REMOTE_HOST}" "command -v helm &>/dev/null"; then
    CURRENT_VERSION=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "helm version --short 2>/dev/null" || echo "unknown")
    echo "⚠ Helm is already installed: ${CURRENT_VERSION}"
    read -p "Do you want to reinstall/upgrade? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Installation cancelled."
        exit 0
    fi
fi
echo

# Detect architecture
echo "Step 3: Detecting system architecture..."
ARCH=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "uname -m")
case "$ARCH" in
    x86_64)
        HELM_ARCH="amd64"
        ;;
    aarch64|arm64)
        HELM_ARCH="arm64"
        ;;
    *)
        echo "ERROR: Unsupported architecture: $ARCH"
        exit 1
        ;;
esac
echo "✓ Architecture: ${ARCH} (Helm: ${HELM_ARCH})"
echo

# Install Helm
echo "Step 4: Installing Helm ${HELM_VERSION}..."
ssh "${REMOTE_USER}@${REMOTE_HOST}" bash << EOF
set -e

# Download Helm
echo "  - Downloading Helm..."
cd /tmp
curl -fsSL -o helm.tar.gz "https://get.helm.sh/helm-${HELM_VERSION}-linux-${HELM_ARCH}.tar.gz"

# Extract
echo "  - Extracting..."
tar -zxf helm.tar.gz

# Try to install to /usr/local/bin with sudo, fallback to ~/bin
echo "  - Installing..."
if sudo -n mv linux-${HELM_ARCH}/helm /usr/local/bin/helm 2>/dev/null && sudo -n chmod +x /usr/local/bin/helm 2>/dev/null; then
    echo "    Installed to /usr/local/bin/helm"
else
    echo "    Cannot use sudo, installing to ~/bin instead..."
    mkdir -p ~/bin
    mv linux-${HELM_ARCH}/helm ~/bin/helm
    chmod +x ~/bin/helm
    echo "    Installed to ~/bin/helm"

    # Add to PATH if not already there
    if ! echo \$PATH | grep -q "\$HOME/bin"; then
        echo 'export PATH=\$HOME/bin:\$PATH' >> ~/.bashrc
        echo "    Added ~/bin to PATH in ~/.bashrc"
    fi
fi

# Cleanup
echo "  - Cleaning up..."
rm -rf linux-${HELM_ARCH} helm.tar.gz

echo "✓ Helm installed successfully"
EOF
echo

# Verify installation
echo "Step 5: Verifying installation..."
INSTALLED_VERSION=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "export PATH=\$HOME/bin:\$PATH; helm version --short" || echo "error")
if [ "$INSTALLED_VERSION" != "error" ]; then
    echo "✓ Helm is working: ${INSTALLED_VERSION}"
else
    echo "✗ Helm installation failed"
    exit 1
fi
echo

# Initialize Helm (add common repos)
echo "Step 6: Adding common Helm repositories..."
ssh "${REMOTE_USER}@${REMOTE_HOST}" bash << 'EOF'
set -e

# Add ~/bin to PATH
export PATH=$HOME/bin:$PATH

# Skip if KUBECONFIG doesn't exist (won't be able to use Helm anyway)
if [ ! -f ~/.kube/config ]; then
    echo "⚠ No kubeconfig found, skipping repository setup"
    exit 0
fi

export KUBECONFIG=~/.kube/config

echo "  - Adding stable repository..."
helm repo add stable https://charts.helm.sh/stable 2>/dev/null || echo "    (already exists)"

echo "  - Adding bitnami repository..."
helm repo add bitnami https://charts.bitnami.com/bitnami 2>/dev/null || echo "    (already exists)"

echo "  - Updating repositories..."
helm repo update

echo "✓ Repositories configured"
EOF
echo

echo "======================================"
echo "Helm Installation Complete!"
echo "======================================"
echo
echo "Installed version: ${INSTALLED_VERSION}"
echo
echo "Next steps:"
echo "  1. Run pre-deployment checks: ./pre-deploy-checks.sh"
echo "  2. Deploy EdgeLake: ./deploy-test.sh"
echo
