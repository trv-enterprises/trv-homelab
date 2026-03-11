#!/bin/bash
# Setup passwordless sudo for current user
# Usage: Run this script, it will prompt for password once to configure passwordless sudo

set -e

# Configuration
REMOTE_HOST="100.74.102.38"
REMOTE_USER="<user>"

echo "======================================"
echo "Setup Passwordless Sudo"
echo "Target: ${REMOTE_HOST}"
echo "User: ${REMOTE_USER}"
echo "======================================"
echo

echo "This will configure passwordless sudo for user: ${REMOTE_USER}"
echo "You will be prompted for your sudo password ONCE to make this change."
echo

# Create the sudoers entry
echo "Creating sudoers configuration..."
ssh -t "${REMOTE_USER}@${REMOTE_HOST}" bash << 'EOF'
set -e

SUDOERS_FILE="/etc/sudoers.d/${USER}-nopasswd"

# Create sudoers entry
echo "Creating sudoers entry for ${USER}..."
echo "${USER} ALL=(ALL) NOPASSWD:ALL" | sudo tee "$SUDOERS_FILE" > /dev/null

# Set correct permissions (required for sudoers files)
sudo chmod 0440 "$SUDOERS_FILE"

# Verify the file was created
if [ -f "$SUDOERS_FILE" ]; then
    echo "✓ Sudoers file created: $SUDOERS_FILE"
    echo "  Content:"
    sudo cat "$SUDOERS_FILE"
else
    echo "✗ Failed to create sudoers file"
    exit 1
fi

# Test passwordless sudo
echo
echo "Testing passwordless sudo..."
if sudo -n true 2>/dev/null; then
    echo "✓ Passwordless sudo is working!"
else
    echo "✗ Passwordless sudo test failed"
    exit 1
fi
EOF

echo
echo "======================================"
echo "Setup Complete!"
echo "======================================"
echo
echo "User ${REMOTE_USER} can now use sudo without a password on ${REMOTE_HOST}"
echo
echo "Next steps:"
echo "  1. Configure insecure registry: ./configure-insecure-registry.sh"
echo "  2. Deploy EdgeLake: ./deploy-test.sh"
echo
