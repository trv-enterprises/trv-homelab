#!/bin/bash

# Install ts-store stats services on trv-srv-001
# Services: system-stats collector, journal-logs collector, mqtt-sink

set -e

echo "=== Installing ts-store Stats Services ==="

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (use sudo)"
    exit 1
fi

# Create directories
mkdir -p /var/run/tsstore
chown <user>:<user> /var/run/tsstore
mkdir -p /var/lib/mqtt-sink
chown <user>:<user> /var/lib/mqtt-sink

# Install tsstore service
cp ../tsstore.service /etc/systemd/system/

# Install stats services
cp system-stats.service /etc/systemd/system/
cp journal-logs.service /etc/systemd/system/
cp mqtt-sink-system-stats.service /etc/systemd/system/

# Reload systemd
systemctl daemon-reload

# Enable services
systemctl enable tsstore system-stats journal-logs mqtt-sink-system-stats

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Start services:"
echo "  sudo systemctl start tsstore"
echo "  sudo systemctl start system-stats"
echo "  sudo systemctl start journal-logs"
echo "  sudo systemctl start mqtt-sink-system-stats"
echo ""
echo "Check status:"
echo "  systemctl status tsstore system-stats journal-logs mqtt-sink-system-stats"
