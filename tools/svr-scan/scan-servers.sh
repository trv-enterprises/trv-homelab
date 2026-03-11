#!/bin/bash
#
# Server Scanner - Scans all Tailscale hosts and generates system reports
#
# Usage: ./scan-servers.sh [output-dir]
#
# Generates reports for each reachable host including:
# - System info (hostname, OS, uptime)
# - CPU and memory usage
# - Disk usage
# - User systemd services
# - Non-standard system services
# - Docker containers
#

set -euo pipefail

# Configuration
SSH_USER="${SSH_USER:-<user>}"
SSH_TIMEOUT=5
OUTPUT_DIR="${1:-$(dirname "$0")/../../svr-rpts}"
TIMESTAMP=$(date +%Y-%m-%d_%H%M%S)
REPORT_DIR="$OUTPUT_DIR/$TIMESTAMP"

# Find tailscale binary
if command -v tailscale &>/dev/null; then
    TAILSCALE="tailscale"
elif [ -x "/Applications/Tailscale.app/Contents/MacOS/Tailscale" ]; then
    TAILSCALE="/Applications/Tailscale.app/Contents/MacOS/Tailscale"
else
    echo "Error: tailscale not found"
    exit 1
fi

# Colors for terminal output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Standard system services to exclude from "non-standard" list
STANDARD_SERVICES=(
    "accounts-daemon" "acpid" "apparmor" "atd" "avahi-daemon" "blk-availability"
    "colord" "console-setup" "cron" "cups" "dbus" "dm-event" "fwupd"
    "gdm" "gdm3" "getty@" "grub-common" "irqbalance" "keyboard-setup"
    "kmod" "lvm2" "ModemManager" "multipathd" "networkd-dispatcher"
    "NetworkManager" "plymouth" "polkit" "power-profiles-daemon" "rsyslog"
    "rtkit-daemon" "setvtrgb" "snapd" "ssh" "sshd" "switcheroo-control"
    "systemd-" "thermald" "udisks2" "ufw" "unattended-upgrades" "upower"
    "user@" "wpa_supplicant" "packagekit" "kerneloops" "apport" "whoopsie"
    "bolt" "lm-sensors" "fancontrol" "smartmontools" "fstrim" "logrotate"
    "apt-daily" "snapd" "motd-news" "e2scrub" "dpkg-db-backup" "man-db"
    "plocate" "update-notifier" "fwupd-refresh" "secureboot-db"
)

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if a service name matches standard services
is_standard_service() {
    local service="$1"
    for std in "${STANDARD_SERVICES[@]}"; do
        if [[ "$service" == "$std"* ]]; then
            return 0
        fi
    done
    return 1
}

# Get Tailscale hosts
get_tailscale_hosts() {
    $TAILSCALE status --json | jq -r '.Peer | to_entries[] | select(.value.Online == true) | "\(.value.TailscaleIPs[0]) \(.value.HostName)"' 2>/dev/null || true
    # Also include self
    $TAILSCALE status --json | jq -r '.Self | "\(.TailscaleIPs[0]) \(.HostName)"' 2>/dev/null || true
}

# Test SSH connectivity
test_ssh() {
    local host="$1"
    ssh -n -o BatchMode=yes -o ConnectTimeout=$SSH_TIMEOUT -o StrictHostKeyChecking=no "$SSH_USER@$host" "echo ok" &>/dev/null
}

# Collect system info from a host
collect_system_info() {
    local host="$1"
    local hostname="$2"
    local report_file="$REPORT_DIR/${hostname}.md"

    log_info "Scanning $hostname ($host)..."

    # Remote script to collect all info
    local remote_script='
#!/bin/bash

echo "## System Information"
echo ""
echo "| Property | Value |"
echo "|----------|-------|"
echo "| Hostname | $(hostname) |"
echo "| OS | $(cat /etc/os-release 2>/dev/null | grep "PRETTY_NAME" | cut -d= -f2 | tr -d \") |"
echo "| Kernel | $(uname -r) |"
echo "| Uptime | $(uptime -p 2>/dev/null || uptime | sed "s/.*up //" | sed "s/,.*//") |"
echo "| Architecture | $(uname -m) |"
echo ""

echo "## Resource Usage"
echo ""

# CPU info
cpu_cores=$(nproc 2>/dev/null || echo "?")
cpu_model=$(cat /proc/cpuinfo 2>/dev/null | grep "model name" | head -1 | cut -d: -f2 | xargs || echo "Unknown")
cpu_load=$(cat /proc/loadavg 2>/dev/null | awk "{print \$1, \$2, \$3}" || echo "? ? ?")

echo "### CPU"
echo ""
echo "| Property | Value |"
echo "|----------|-------|"
echo "| Model | $cpu_model |"
echo "| Cores | $cpu_cores |"
echo "| Load (1/5/15 min) | $cpu_load |"
echo ""

# Memory info
if command -v free &>/dev/null; then
    mem_total=$(free -h | awk "/^Mem:/ {print \$2}")
    mem_used=$(free -h | awk "/^Mem:/ {print \$3}")
    mem_avail=$(free -h | awk "/^Mem:/ {print \$7}")
    mem_pct=$(free | awk "/^Mem:/ {printf \"%.1f%%\", \$3/\$2 * 100}")

    echo "### Memory"
    echo ""
    echo "| Property | Value |"
    echo "|----------|-------|"
    echo "| Total | $mem_total |"
    echo "| Used | $mem_used ($mem_pct) |"
    echo "| Available | $mem_avail |"
    echo ""
fi

# Disk info
echo "### Disk Usage"
echo ""
echo "| Filesystem | Size | Used | Avail | Use% | Mounted on |"
echo "|------------|------|------|-------|------|------------|"
df -h 2>/dev/null | grep -E "^/dev/" | while read fs size used avail pct mount; do
    echo "| $fs | $size | $used | $avail | $pct | $mount |"
done
echo ""

# User services (systemd --user)
echo "## User Services (systemd --user)"
echo ""
user_services=$(systemctl --user list-units --type=service --state=running --no-pager --no-legend 2>/dev/null | awk "{print \$1}" || true)
if [ -n "$user_services" ]; then
    echo "| Service | Status |"
    echo "|---------|--------|"
    echo "$user_services" | while read svc; do
        [ -n "$svc" ] && echo "| $svc | running |"
    done
else
    echo "_No user services found_"
fi
echo ""

# Non-standard system services
echo "## Non-Standard System Services"
echo ""
echo "| Service | Status | Description |"
echo "|---------|--------|-------------|"
systemctl list-units --type=service --state=running --no-pager --no-legend 2>/dev/null | while read line; do
    svc=$(echo "$line" | awk "{print \$1}" | sed "s/.service$//")
    desc=$(echo "$line" | awk "{for(i=5;i<=NF;i++) printf \"%s \", \$i; print \"\"}" | xargs)

    # Skip standard services
    is_std=0
    for std in accounts-daemon acpid apparmor atd avahi-daemon blk-availability \
               colord console-setup cron cups dbus dm-event fwupd gdm gdm3 \
               getty@ grub-common irqbalance keyboard-setup kmod lvm2 \
               ModemManager multipathd networkd-dispatcher NetworkManager \
               plymouth polkit power-profiles-daemon rsyslog rtkit-daemon \
               setvtrgb snapd ssh sshd switcheroo-control systemd- thermald \
               udisks2 ufw unattended-upgrades upower user@ wpa_supplicant \
               packagekit kerneloops apport whoopsie bolt lm-sensors fancontrol \
               smartmontools fstrim logrotate apt-daily motd-news e2scrub \
               dpkg-db-backup man-db plocate update-notifier fwupd-refresh \
               secureboot-db tailscaled containerd docker snapd; do
        if [[ "$svc" == "$std"* ]]; then
            is_std=1
            break
        fi
    done

    [ $is_std -eq 0 ] && echo "| $svc | running | $desc |"
done
echo ""

# Docker containers
echo "## Docker Containers"
echo ""
if command -v docker &>/dev/null && docker ps &>/dev/null 2>&1; then
    containers=$(docker ps --format "{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || true)
    if [ -n "$containers" ]; then
        echo "| Name | Image | Status | Ports |"
        echo "|------|-------|--------|-------|"
        echo "$containers" | while IFS=$'\''\t'\'' read name image status ports; do
            ports_clean=$(echo "$ports" | sed "s/0.0.0.0://g" | sed "s/:::/ /g" | head -c 50)
            echo "| $name | $image | $status | $ports_clean |"
        done
    else
        echo "_No running containers_"
    fi
else
    echo "_Docker not available or not accessible_"
fi
echo ""

# Docker volumes
if command -v docker &>/dev/null && docker volume ls &>/dev/null 2>&1; then
    volumes=$(docker volume ls --format "{{.Name}}" 2>/dev/null | head -20 || true)
    if [ -n "$volumes" ]; then
        echo "### Docker Volumes (first 20)"
        echo ""
        echo "| Volume Name |"
        echo "|-------------|"
        echo "$volumes" | while read vol; do
            echo "| $vol |"
        done
        echo ""
    fi
fi
'

    # Execute remote script (-n prevents stdin consumption in while loop)
    if ssh -n -o BatchMode=yes -o ConnectTimeout=$SSH_TIMEOUT -o StrictHostKeyChecking=no "$SSH_USER@$host" "$remote_script" > "$report_file" 2>/dev/null; then
        # Add header
        local header="# Server Report: $hostname

**IP Address:** $host
**Scanned:** $(date '+%Y-%m-%d %H:%M:%S')
**Scanner:** $(hostname)

---

"
        echo "$header$(cat "$report_file")" > "$report_file"
        log_info "  Report saved: $report_file"
        return 0
    else
        log_error "  Failed to collect info from $hostname"
        return 1
    fi
}

# Generate summary report
generate_summary() {
    local summary_file="$REPORT_DIR/00-SUMMARY.md"

    log_info "Generating summary report..."

    cat > "$summary_file" << EOF
# Server Scan Summary

**Scan Time:** $(date '+%Y-%m-%d %H:%M:%S')
**Scanner:** $(hostname)
**SSH User:** $SSH_USER

---

## Hosts Scanned

| Hostname | IP Address | Status |
|----------|------------|--------|
EOF

    # Add host status
    while IFS=' ' read -r ip hostname; do
        [ -z "$ip" ] && continue
        if [ -f "$REPORT_DIR/${hostname}.md" ]; then
            echo "| [$hostname](${hostname}.md) | $ip | Scanned |" >> "$summary_file"
        else
            echo "| $hostname | $ip | Unreachable |" >> "$summary_file"
        fi
    done <<< "$ALL_HOSTS"

    cat >> "$summary_file" << EOF

---

## Quick Stats

EOF

    # Collect quick stats from each report
    for report in "$REPORT_DIR"/*.md; do
        [ "$(basename "$report")" = "00-SUMMARY.md" ] && continue
        [ ! -f "$report" ] && continue

        hostname=$(basename "$report" .md)
        mem_line=$(grep -A3 "### Memory" "$report" 2>/dev/null | grep "Used" | head -1 || echo "")
        disk_line=$(grep -E "^\| /dev/" "$report" 2>/dev/null | head -1 || echo "")
        container_count=$(grep -c "^\| [a-z]" "$report" 2>/dev/null | tail -1 || echo "0")

        if [ -n "$mem_line" ]; then
            mem_used=$(echo "$mem_line" | awk -F'|' '{print $3}' | xargs)
            echo "**$hostname**: Memory: $mem_used" >> "$summary_file"
        fi
    done

    log_info "Summary saved: $summary_file"
}

# Main
main() {
    echo ""
    echo "=================================="
    echo "  Server Scanner"
    echo "=================================="
    echo ""

    # Create output directory
    mkdir -p "$REPORT_DIR"
    log_info "Output directory: $REPORT_DIR"
    echo ""

    # Get Tailscale hosts
    log_info "Discovering Tailscale hosts..."
    ALL_HOSTS=$(get_tailscale_hosts)

    if [ -z "$ALL_HOSTS" ]; then
        log_error "No Tailscale hosts found. Is Tailscale running?"
        exit 1
    fi

    host_count=$(echo "$ALL_HOSTS" | wc -l | xargs)
    log_info "Found $host_count hosts"
    echo ""

    # Scan each host
    scanned=0
    failed=0

    while IFS=' ' read -r ip hostname; do
        [ -z "$ip" ] && continue

        if test_ssh "$ip"; then
            if collect_system_info "$ip" "$hostname"; then
                scanned=$((scanned + 1))
            else
                failed=$((failed + 1))
            fi
        else
            log_warn "Cannot SSH to $hostname ($ip) - skipping"
            failed=$((failed + 1))
        fi
    done <<< "$ALL_HOSTS"

    echo ""

    # Generate summary
    generate_summary

    echo ""
    echo "=================================="
    echo "  Scan Complete"
    echo "=================================="
    echo ""
    log_info "Scanned: $scanned hosts"
    [ $failed -gt 0 ] && log_warn "Failed/Skipped: $failed hosts"
    log_info "Reports: $REPORT_DIR"
    echo ""

    # Create latest symlink
    ln -sfn "$TIMESTAMP" "$OUTPUT_DIR/latest"
    log_info "Latest symlink: $OUTPUT_DIR/latest"
}

main "$@"
