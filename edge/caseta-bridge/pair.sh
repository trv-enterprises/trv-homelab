#!/bin/bash
# Caseta L-BDG2 pairing helper
# Generates TLS certificates for LEAP protocol communication.
#
# Usage: ./pair.sh <bridge-ip>
#
# Before running:
#   1. Find the L-BDG2 on your network (check router DHCP leases)
#   2. Have physical access to the bridge
#
# When prompted, press the small black button on the back of the L-BDG2.

set -euo pipefail

BRIDGE_HOST="${1:?Usage: ./pair.sh <bridge-ip>}"
CERT_DIR="/etc/caseta-bridge/certs"

if ! command -v lap-pair &>/dev/null; then
    echo "Installing pylutron-caseta CLI tools..."
    pip3 install 'pylutron-caseta[cli]>=0.27.0'
fi

sudo mkdir -p "$CERT_DIR"

echo ""
echo "=== Caseta Bridge Pairing ==="
echo "Bridge IP: $BRIDGE_HOST"
echo ""
echo "Press the small black button on the BACK of the Caseta bridge now..."
echo ""

lap-pair "$BRIDGE_HOST"

# lap-pair writes certs to ~/.config/pylutron_caseta/
# v0.27+ uses <bridge-ip>.key naming; older versions use caseta.key
SRC_DIR="$HOME/.config/pylutron_caseta"

if [ ! -d "$SRC_DIR" ]; then
    echo "ERROR: Pairing did not generate certificates in $SRC_DIR"
    exit 1
fi

if [ -f "$SRC_DIR/$BRIDGE_HOST.key" ]; then
    # pylutron-caseta v0.27+ naming
    sudo cp "$SRC_DIR/$BRIDGE_HOST.key" "$CERT_DIR/client.key"
    sudo cp "$SRC_DIR/$BRIDGE_HOST.crt" "$CERT_DIR/client.crt"
    sudo cp "$SRC_DIR/$BRIDGE_HOST-bridge.crt" "$CERT_DIR/ca.crt"
elif [ -f "$SRC_DIR/caseta.key" ]; then
    # Older naming
    sudo cp "$SRC_DIR/caseta.key" "$CERT_DIR/client.key"
    sudo cp "$SRC_DIR/caseta.crt" "$CERT_DIR/client.crt"
    sudo cp "$SRC_DIR/caseta-bridge.crt" "$CERT_DIR/ca.crt"
else
    echo "ERROR: Could not find certificate files in $SRC_DIR"
    ls -la "$SRC_DIR"
    exit 1
fi
sudo chmod 600 "$CERT_DIR"/*.key
sudo chmod 644 "$CERT_DIR"/*.crt

echo ""
echo "Certificates saved to $CERT_DIR:"
ls -la "$CERT_DIR"
echo ""
echo "Update config.yaml with bridge_host: $BRIDGE_HOST"
