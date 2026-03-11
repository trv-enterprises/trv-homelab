# EdgeLake Operator Deployment on Jetson Nano via Open Horizon

This guide covers deploying an EdgeLake operator on a Jetson Nano using Open Horizon.

## Prerequisites

- Jetson Nano with Ubuntu/JetPack installed
- Network connectivity to OH Hub (<hub-tailscale-ip>)
- Tailscale installed and connected (IP: <jetson-tailscale-ip>)
- SSH access to Jetson

## Architecture

```
OH Hub (<hub-tailscale-ip>)          Jetson Nano (<jetson-tailscale-ip>)
├── Exchange (:3090)            ├── OH Agent
├── CSS (:9443)        <----    ├── EdgeLake Operator
├── Master (:32048)             │   ├── TCP: 32448
└── Query (:32349)              │   ├── REST: 32449
                                │   └── Broker: 32450
Artifactory (:8081)             └── ARM64 Image
└── edgelake:arm64-latest
```

## Phase 1: Build ARM64 EdgeLake Image

### Option A: Build on Jetson (Recommended)

SSH to the Jetson Nano:
```bash
ssh <user>@<jetson-tailscale-ip>
```

Clone EdgeLake repository and build:
```bash
# Clone the repo
git clone https://github.com/EdgeLake/EdgeLake.git ~/EdgeLake
cd ~/EdgeLake

# Build ARM64 image
docker build -t <hub-tailscale-ip>:8081/trve-repo-local/edgelake:arm64-latest .

# Login to Artifactory
docker login <hub-tailscale-ip>:8081

# Push the image
docker push <hub-tailscale-ip>:8081/trve-repo-local/edgelake:arm64-latest
```

### Option B: Cross-compile (Alternative)

If cross-compiling from another machine with Docker buildx:
```bash
docker buildx build --platform linux/arm64 \
  -t <hub-tailscale-ip>:8081/trve-repo-local/edgelake:arm64-latest \
  --push .
```

## Phase 2: Install Open Horizon Agent on Jetson

### 2.1 Install Docker (if needed)

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
# Log out and back in for group changes
```

### 2.2 Configure Artifactory Access

Add insecure registry (Artifactory without TLS):
```bash
sudo tee /etc/docker/daemon.json <<EOF
{
  "insecure-registries": ["<hub-tailscale-ip>:8081"]
}
EOF

sudo systemctl restart docker
```

### 2.3 Download and Install OH Agent

```bash
# Get agent installer from CSS
curl -sSL http://<hub-tailscale-ip>:9443/api/v1/objects/IBM/agent_files/agent-install.sh -o agent-install.sh
chmod +x agent-install.sh

# Configure environment
export HZN_EXCHANGE_URL=http://<hub-tailscale-ip>:3090/v1
export HZN_FSS_CSSURL=http://<hub-tailscale-ip>:9443
export HZN_ORG_ID=myorg
export HZN_EXCHANGE_USER_AUTH=myorg/admin:CHANGEME

# Install agent (uses packages from CSS)
sudo -E ./agent-install.sh -i 'css:' -k css: -c css: -p ''
```

### 2.4 Verify Installation

```bash
hzn version
hzn node list
```

Expected output shows `configstate.state: "configured"` and no errors.

## Phase 3: Publish ARM64 Service to Exchange

This is done from the OH Hub (<hub-tailscale-ip>).

### 3.1 Sync files to hub

From your local machine:
```bash
cd /path/to/trv-edgelake-infra
make -f Makefile.oh sync
```

### 3.2 Publish ARM64 service

SSH to the hub:
```bash
ssh <user>@<hub-tailscale-ip>
cd /home/<user>/trv-edgelake-infra

# Set credentials
export HZN_ORG_ID=myorg
export HZN_EXCHANGE_USER_AUTH=myorg/admin:CHANGEME

# Publish ARM64 service for Jetson
make -f Makefile.oh oh-publish-all \
  ARCH=arm64 \
  CONFIG_PROFILE=operator_jetson \
  SERVICE_VERSION=1.4.0
```

### 3.3 Verify service published

```bash
hzn exchange service list --org=myorg | grep arm64
```

## Phase 4: Register Jetson with Open Horizon

### 4.1 Copy node policy to Jetson

```bash
# From local machine
scp oh-services/operator/node.policy.jetson.json \
  <user>@<jetson-tailscale-ip>:/home/<user>/node.policy.json
```

### 4.2 Register the node

On the Jetson:
```bash
export HZN_ORG_ID=myorg
export HZN_EXCHANGE_USER_AUTH=myorg/admin:CHANGEME

hzn register --name=trv-jetson-nano --policy=/home/<user>/node.policy.json
```

### 4.3 Monitor agreement formation

```bash
watch hzn agreement list
```

Wait for an agreement to form (may take 1-2 minutes).

## Phase 5: Verify Deployment

### 5.1 Check container is running

```bash
docker ps | grep edgelake
```

### 5.2 Test EdgeLake REST API

```bash
# From Jetson
curl http://localhost:32449/get/status

# From any Tailscale machine
curl http://<jetson-tailscale-ip>:32449/get/status
```

### 5.3 Verify blockchain registration

From any machine with network access to the master:
```bash
curl -s -H "command: blockchain get operator" http://<hub-tailscale-ip>:32049 | jq .
```

You should see the Jetson operator listed with:
- IP: <jetson-tailscale-ip>
- Port: 32448
- Name: edgelake-operator-jetson

### 5.4 CLI Access (via helper script)

OH containers don't support TTY. Create a helper script:
```bash
cat > ~/edgelake-cli.sh << 'EOF'
#!/bin/bash
CONTAINER=$(docker ps -q --filter "name=edgelake")
if [ -z "$CONTAINER" ]; then
    echo "No EdgeLake container running"
    exit 1
fi
docker exec $CONTAINER python3 /app/edgelake_agent "$@"
EOF
chmod +x ~/edgelake-cli.sh

# Usage
~/edgelake-cli.sh "get status"
~/edgelake-cli.sh "blockchain get operator"
```

## Troubleshooting

### No agreement forms

1. Check node policy matches service constraints:
   ```bash
   hzn policy list
   ```

2. Verify service is published for arm64:
   ```bash
   hzn exchange service list --org=myorg | grep arm64
   ```

3. Check agent logs:
   ```bash
   sudo journalctl -u horizon -f
   ```

### Container won't start

1. Check if image can be pulled:
   ```bash
   docker pull <hub-tailscale-ip>:8081/trve-repo-local/edgelake:arm64-latest
   ```

2. Check agreement errors:
   ```bash
   hzn agreement list | jq '.[]|.agreement_execution_start_time,.agreement_terminated_time'
   ```

### Port conflicts

If ports 32448/32449/32450 are in use, update the Jetson config:
```bash
# Edit oh-services/operator/configurations/operator_jetson.env
# Change ports, then re-publish service
```

## Configuration Reference

| Setting | Value |
|---------|-------|
| Jetson Tailscale IP | <jetson-tailscale-ip> |
| OH Organization | myorg |
| TCP Port | 32448 |
| REST Port | 32449 |
| Broker Port | 32450 |
| Master (LEDGER_CONN) | <hub-tailscale-ip>:32048 |
| Image | <hub-tailscale-ip>:8081/trve-repo-local/edgelake:arm64-latest |
| Architecture | arm64 |
| Config Profile | operator_jetson |

## Rollback

### Unregister node
```bash
hzn unregister -f
```

### Remove service from Exchange (from hub)
```bash
hzn exchange service remove myorg/service-edgelake-operator_1.4.0_arm64 -f
hzn exchange deployment removepolicy myorg/policy-service-edgelake-operator_1.4.0 -f
```

## Files Modified

- `Makefile.oh` - Added ARCH and CONFIG_PROFILE variables
- `oh-services/operator/configurations/operator_jetson.env` - Jetson-specific config
- `oh-services/operator/node.policy.jetson.json` - Jetson node policy
