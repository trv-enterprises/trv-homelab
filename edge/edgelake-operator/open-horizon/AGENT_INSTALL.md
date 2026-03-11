# Open Horizon Agent Installation Guide

This guide covers installing and configuring the Open Horizon agent on edge devices for EdgeLake deployment.

## Prerequisites

### Hardware Requirements
- x86_64 or ARM64 architecture
- Minimum 2GB RAM
- Minimum 20GB disk space
- Network connectivity to OH hub (<hub-tailscale-ip>)

### Software Requirements
- Ubuntu 20.04+ / Debian 10+ / RHEL 8+
- Docker or Podman installed
- SSH access to device
- sudo/root privileges

## Installation Steps

### Step 1: Install Docker (if not already installed)

```bash
# Update package index
sudo apt-get update

# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Add user to docker group (logout/login required)
sudo usermod -aG docker $USER

# Verify Docker installation
docker version
docker ps
```

### Step 2: Download OH Agent Installation Script

From the OH hub or download directly:

```bash
# Option A: Download from OH hub
wget http://<hub-tailscale-ip>/agent-install.sh
chmod +x agent-install.sh

# Option B: Use curl
curl -sSL http://<hub-tailscale-ip>/agent-install.sh -o agent-install.sh
chmod +x agent-install.sh
```

### Step 3: Set Environment Variables

```bash
# OH organization ID
export HZN_ORG_ID=myorg

# OH Exchange URL
export HZN_EXCHANGE_URL=http://<hub-tailscale-ip>:3090/v1

# OH CSS (Sync Service) URL
export HZN_FSS_CSSURL=http://<hub-tailscale-ip>:9443

# Agent authentication (create on hub first)
export HZN_EXCHANGE_USER_AUTH=<device-id>:<device-token>

# Or use admin credentials for initial testing
export HZN_EXCHANGE_USER_AUTH=admin:<admin-password>
```

### Step 4: Install OH Agent

```bash
# Install agent only (no registration yet)
sudo -E ./agent-install.sh -i 'css:' -k css: -c css: -p ''

# Verify installation
hzn version
hzn node list
```

Expected output:
```json
{
  "id": "",
  "organization": null,
  "pattern": "",
  "name": "",
  "nodeType": "device",
  "token_last_valid_time": "",
  "token_valid": null,
  "ha": null,
  "configstate": {
    "state": "unconfigured",
    "last_update_time": ""
  }
}
```

### Step 5: Verify Connectivity to Hub

```bash
# Test Exchange connection
hzn exchange status

# List organizations (verify access)
hzn exchange org list

# List available services (should see EdgeLake services after publishing)
hzn exchange service list --org=$HZN_ORG_ID
```

## Device Registration

### Option 1: Using Node Policy (Recommended)

This is the recommended method for policy-based deployment.

```bash
# Create node policy file
cat > node-policy.json << 'EOF'
{
  "properties": [
    {"name": "purpose", "value": "edgelake"},
    {"name": "openhorizon.allowPrivileged", "value": true},
    {"name": "device-location", "value": "datacenter-1"},
    {"name": "device-id", "value": "operator-prod-1"}
  ],
  "constraints": []
}
EOF

# Register device
hzn register \
  --org=$HZN_ORG_ID \
  --user-pw=$HZN_EXCHANGE_USER_AUTH \
  --name=operator-prod-1 \
  --policy=node-policy.json

# Verify registration
hzn node list
hzn policy list
```

### Option 2: Using Pattern (Alternative)

If using patterns instead of policies:

```bash
hzn register \
  --org=$HZN_ORG_ID \
  --user-pw=$HZN_EXCHANGE_USER_AUTH \
  --pattern=pattern-edgelake-operator
```

## Monitoring Deployment

### Check Agreement Status

Agreements are formed when device policy matches deployment policy:

```bash
# Monitor agreement formation (may take 1-2 minutes)
watch -n 2 hzn agreement list

# Once agreement is formed:
hzn agreement list
```

Expected output (after agreement):
```json
[
  {
    "name": "Policy for myorg/policy-service-edgelake-operator_1.3.5 merged with myorg/<device-id> policy",
    "current_agreement_id": "<agreement-id>",
    "consumer_id": "<agbot-id>",
    "agreement_creation_time": "2025-11-17 12:34:56 -0800 PST",
    "agreement_accepted_time": "2025-11-17 12:35:10 -0800 PST",
    "agreement_finalized_time": "2025-11-17 12:35:25 -0800 PST",
    "agreement_execution_start_time": "2025-11-17 12:35:30 -0800 PST",
    "agreement_data_received_time": "",
    "agreement_protocol": "Basic",
    "workload_to_run": {
      "url": "service-edgelake-operator",
      "org": "myorg",
      "version": "1.3.5",
      "arch": "amd64"
    }
  }
]
```

### Check Service Status

```bash
# List running services
hzn service list

# Check Docker containers
docker ps | grep edgelake

# View container logs
docker logs -f <container-name>
```

### Check EdgeLake API

Once the container is running:

```bash
# Test REST API
curl http://localhost:32149/status

# Test TCP connection to master
telnet <hub-tailscale-ip> 32048
```

## Troubleshooting

### Agent Not Installing

**Issue**: `agent-install.sh` fails

```bash
# Check system requirements
cat /etc/os-release
uname -m

# Check Docker is running
sudo systemctl status docker

# Try manual installation
sudo apt-get install horizon horizon-cli
```

### Cannot Connect to Exchange

**Issue**: `hzn exchange status` fails

```bash
# Verify hub is reachable
ping <hub-tailscale-ip>
curl http://<hub-tailscale-ip>:3090/v1/admin/version

# Check firewall rules
sudo iptables -L -n | grep 3090

# Verify environment variables
echo $HZN_EXCHANGE_URL
echo $HZN_FSS_CSSURL
```

### No Agreement Formed

**Issue**: `hzn agreement list` shows empty

```bash
# Check node policy
hzn policy list

# Check if deployment policy exists
hzn exchange deployment listpolicy --org=$HZN_ORG_ID

# Check event log for errors
hzn eventlog list | tail -20

# Verify service is published
hzn exchange service list --org=$HZN_ORG_ID | grep edgelake

# Check agbot status (on hub)
ssh <user>@<hub-tailscale-ip> "hzn agbot status"
```

### Container Not Starting

**Issue**: Agreement formed but container not running

```bash
# Check container status
docker ps -a | grep edgelake

# View container logs
docker logs <container-id>

# Check if image is accessible
docker pull <hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest

# Verify privileged mode is allowed
docker inspect <container-id> | grep -i privileged

# Check service logs from OH
hzn service log <service-name>
```

### Image Pull Errors

**Issue**: Cannot pull image from registry

```bash
# Test registry access
curl http://<hub-tailscale-ip>:5000/v2/_catalog

# Configure insecure registry (if using HTTP)
sudo vi /etc/docker/daemon.json
# Add:
# {
#   "insecure-registries": ["<hub-tailscale-ip>:5000"]
# }

# Restart Docker
sudo systemctl restart docker

# Try manual pull
docker pull <hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest
```

### Permission Denied Errors

**Issue**: Docker permission errors

```bash
# Add user to docker group
sudo usermod -aG docker $USER

# Logout and login again, or:
newgrp docker

# Verify
docker ps
```

## Device Management

### Update Configuration

To update EdgeLake configuration:

```bash
# Unregister device
hzn unregister -f

# Wait for container to stop
docker ps -a

# Update node policy (if needed)
vi node-policy.json

# Re-register
hzn register --name=operator-prod-1 --policy=node-policy.json
```

### Unregister Device

```bash
# Unregister and remove all services
hzn unregister -f

# Verify container is stopped
docker ps -a | grep edgelake

# Remove stopped containers
docker container prune -f

# Remove volumes (WARNING: Deletes data!)
docker volume ls | grep edgelake
docker volume prune -f
```

### View Device in Exchange

From hub or local machine with hzn CLI:

```bash
# List all nodes
hzn exchange node list --org=$HZN_ORG_ID

# Show specific node
hzn exchange node show <node-id> --org=$HZN_ORG_ID

# Show node policy
hzn exchange node listpolicy <node-id> --org=$HZN_ORG_ID
```

## Production Deployment Checklist

Before deploying to production:

- [ ] Docker installed and user in docker group
- [ ] OH agent installed and registered
- [ ] Network connectivity to hub (<hub-tailscale-ip>)
- [ ] Registry accessible (<hub-tailscale-ip>:5000)
- [ ] Firewall allows ports: 32148 (TCP), 32149 (REST)
- [ ] Node policy matches deployment constraints
- [ ] Agreement formed successfully
- [ ] EdgeLake container running
- [ ] EdgeLake API responding (port 32149)
- [ ] Connection to master node verified (<hub-tailscale-ip>:32048)
- [ ] Monitoring configured (if enabled)

## Quick Reference

### Common Commands

```bash
# Check agent status
hzn node list

# List agreements
hzn agreement list

# List running services
hzn service list

# View event log
hzn eventlog list

# View node policy
hzn policy list

# Unregister device
hzn unregister -f

# Check Exchange connection
hzn exchange status

# List all nodes in org (from hub)
hzn exchange node list --org=$HZN_ORG_ID
```

### Service-Specific Commands

```bash
# View EdgeLake logs
docker logs -f $(docker ps -q --filter "ancestor=<hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest")

# Attach to EdgeLake CLI
docker attach $(docker ps -q --filter "ancestor=<hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest")
# Press Ctrl-D to detach (not Ctrl-C!)

# Test EdgeLake REST API
curl http://localhost:32149/status

# Check EdgeLake volumes
docker volume ls | grep edgelake
```

## Device Configuration for <edge-srv-014-tailscale-ip>

Specific setup for the target device:

```bash
# SSH to device
ssh <user>@<edge-srv-014-tailscale-ip>

# Install Docker (if needed)
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker <user>

# Install OH agent
export HZN_ORG_ID=myorg
export HZN_EXCHANGE_URL=http://<hub-tailscale-ip>:3090/v1
export HZN_FSS_CSSURL=http://<hub-tailscale-ip>:9443
export HZN_EXCHANGE_USER_AUTH=admin:<password>

wget http://<hub-tailscale-ip>/agent-install.sh
chmod +x agent-install.sh
sudo -E ./agent-install.sh -i 'css:' -k css: -c css: -p ''

# Register with policy
cat > node-policy.json << 'EOF'
{
  "properties": [
    {"name": "purpose", "value": "edgelake"},
    {"name": "openhorizon.allowPrivileged", "value": true},
    {"name": "device-location", "value": "<edge-srv-014-tailscale-ip>"},
    {"name": "device-name", "value": "operator-prod-1"}
  ],
  "constraints": []
}
EOF

hzn register --name=operator-prod-1 --policy=node-policy.json

# Monitor
watch hzn agreement list
```

## Support

For issues:
- Check OH Exchange logs on hub: `docker logs <exchange-container>`
- Check agbot logs on hub: `docker logs <agbot-container>`
- Review deployment plan: [OH_DEPLOYMENT_PLAN.md](../OH_DEPLOYMENT_PLAN.md)
- EdgeLake docs: `/path/to/documentation`
