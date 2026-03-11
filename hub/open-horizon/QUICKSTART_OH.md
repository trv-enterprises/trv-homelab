# Open Horizon EdgeLake Quick Start

Fast-track guide to deploying EdgeLake operator to device **<edge-srv-014-tailscale-ip>** via Open Horizon.

## Prerequisites

- OH Hub running at <hub-tailscale-ip> with Exchange, CSS, AgBot
- EdgeLake Master node running at <hub-tailscale-ip>:32048
- Docker registry at <hub-tailscale-ip>:5000 with edgelake-mcp image
- Target device <edge-srv-014-tailscale-ip> with Docker installed

## Step 1: Publish Service to OH Exchange (Hub Side)

On your local machine or hub:

```bash
cd /path/to/trv-edgelake-infra

# Set credentials
export HZN_ORG_ID=myorg
export HZN_EXCHANGE_USER_AUTH=myorg/admin:CHANGEME

# Validate configuration
make -f Makefile.oh oh-check-vars

# Test connection to Exchange
make -f Makefile.oh oh-test-connection

# Publish everything (service + policies)
make -f Makefile.oh oh-publish-all

# Verify publication
make -f Makefile.oh oh-list-services
make -f Makefile.oh oh-list-deployments
```

**What this does:**
- Generates service definition for EdgeLake operator
- Generates deployment policy from `operator_production.env`
- Publishes service to OH Exchange
- Publishes service and deployment policies

## Step 2: Install OH Agent on Edge Device

SSH to the device:

```bash
ssh <user>@<edge-srv-014-tailscale-ip>
```

On the device:

```bash
# Set environment
export HZN_ORG_ID=myorg
export HZN_EXCHANGE_URL=http://<hub-tailscale-ip>:3090/v1
export HZN_FSS_CSSURL=http://<hub-tailscale-ip>:9443
export HZN_EXCHANGE_USER_AUTH=myorg/admin:CHANGEME

# Download agent install script
wget http://<hub-tailscale-ip>/agent-install.sh
chmod +x agent-install.sh

# Install agent
sudo -E ./agent-install.sh -i 'css:' -k css: -c css: -p ''

# Verify installation
hzn version
hzn node list
hzn exchange status
```

## Step 3: Register Device with Node Policy

Still on the device (<edge-srv-014-tailscale-ip>):

```bash
# Create node policy
cat > node-policy.json << 'EOF'
{
  "properties": [
    {"name": "purpose", "value": "edgelake"},
    {"name": "openhorizon.allowPrivileged", "value": true},
    {"name": "device-name", "value": "operator-prod-1"}
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
```

## Step 4: Monitor Deployment

Wait for agreement formation (1-2 minutes):

```bash
# Watch for agreement (Ctrl-C to exit)
watch -n 2 hzn agreement list

# Once agreement appears, check service
hzn service list

# Check Docker container
docker ps | grep edgelake
```

Expected container:
```
CONTAINER ID   IMAGE                                               COMMAND              CREATED         STATUS
abc123def456   <hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest       "/app/entrypoint.sh" 30 seconds ago  Up 28 seconds
```

## Step 5: Verify EdgeLake Operator

On the device:

```bash
# Get container name/ID
CONTAINER_ID=$(docker ps -q --filter "ancestor=<hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest")

# View logs
docker logs -f $CONTAINER_ID

# Test REST API
curl http://localhost:32149/status

# Test connection to master
curl http://<hub-tailscale-ip>:32049/status

# Check volumes
docker volume ls | grep edgelake
```

### Accessing EdgeLake CLI

**Note**: Open Horizon containers have long names (prefixed with agreement ID) and don't support TTY by default, making `docker attach` difficult.

**Option 1: Use the helper script** (recommended)
```bash
# On edge device, copy the helper script
scp /path/to/oh-services/edgelake-cli.sh <user>@<edge-srv-014-tailscale-ip>:~

# Run EdgeLake commands
./edgelake-cli.sh 'get status'
./edgelake-cli.sh 'get processes'
./edgelake-cli.sh 'blockchain get operator'

# Or open bash shell
./edgelake-cli.sh shell
```

**Option 2: Direct docker exec** (no TTY)
```bash
CONTAINER_ID=$(docker ps -q --filter "ancestor=<hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest")
docker exec -i $CONTAINER_ID bash
```

Expected volumes:
```
edgelake-operator-anylog
edgelake-operator-blockchain
edgelake-operator-data
edgelake-operator-local-scripts
```

## Troubleshooting

### No Agreement Formed

```bash
# Check event log
hzn eventlog list | tail -20

# Verify service exists
hzn exchange service list --org=$HZN_ORG_ID | grep edgelake

# Check deployment policy
hzn exchange deployment listpolicy --org=$HZN_ORG_ID | grep edgelake

# Verify node policy
hzn policy list
```

### Container Not Starting

```bash
# Check all containers (including stopped)
docker ps -a | grep edgelake

# View container logs
docker logs $(docker ps -aq | head -1)

# Test image pull manually
docker pull <hub-tailscale-ip>:5000/edgelake-mcp:amd64-latest

# Check Docker daemon config for insecure registry
cat /etc/docker/daemon.json
```

If registry is insecure:
```bash
sudo vi /etc/docker/daemon.json
# Add:
{
  "insecure-registries": ["<hub-tailscale-ip>:5000"]
}

sudo systemctl restart docker
```

### Image Pull Fails

```bash
# Test registry access
curl http://<hub-tailscale-ip>:5000/v2/_catalog

# Test network to hub
ping <hub-tailscale-ip>
telnet <hub-tailscale-ip> 5000
```

## Configuration Updates

To change EdgeLake configuration:

### Option 1: Update and Republish

```bash
# On local machine
cd /path/to/trv-edgelake-infra

# Edit configuration
vi oh-services/operator/configurations/operator_production.env

# Increment version
export SERVICE_VERSION=1.3.6

# Republish
make -f Makefile.oh oh-publish-all

# Device will auto-update based on policy
```

### Option 2: Unregister and Re-register Device

```bash
# On device
hzn unregister -f

# Wait for container to stop
docker ps -a

# Update node policy if needed
vi node-policy.json

# Re-register
hzn register --name=operator-prod-1 --policy=node-policy.json
```

## Next Steps

After successful deployment:

1. **Verify operator connectivity to master:**
   ```bash
   # From operator (<edge-srv-014-tailscale-ip>)
   telnet <hub-tailscale-ip> 32048
   ```

2. **Check operator registered in master:**
   ```bash
   # SSH to master (<hub-tailscale-ip>)
   # Access master CLI and check for operator node
   ```

3. **Deploy additional operators:**
   - Update `NODE_NAME` in configuration
   - Publish new version or use same service
   - Register additional devices with unique names

4. **Set up monitoring:**
   - Enable `MONITOR_NODES=true` in configuration
   - Configure Grafana dashboards

## Files Created

```
trv-edgelake-infra/
├── Makefile.oh                              # Main Makefile for OH operations
├── QUICKSTART_OH.md                         # This file
├── OH_DEPLOYMENT_PLAN.md                    # Full deployment plan
└── oh-services/
    ├── README.md                            # Detailed OH services documentation
    ├── AGENT_INSTALL.md                     # Agent installation guide
    └── operator/
        ├── configurations/
        │   └── operator_production.env      # EdgeLake configuration
        ├── service.definition.json.template # OH service template
        ├── service.policy.json              # Service constraints
        ├── node.policy.json                 # Node registration policy
        └── create_deployment_policy.py      # Policy generator script
```

## Command Reference

```bash
# Publishing
make -f Makefile.oh oh-publish-all           # Publish everything
make -f Makefile.oh oh-check-vars            # Validate config
make -f Makefile.oh oh-list-services         # List services
make -f Makefile.oh oh-show-service          # Show service details

# Device (on <edge-srv-014-tailscale-ip>)
hzn node list                                # Device status
hzn agreement list                           # Agreement status
hzn service list                             # Running services
hzn eventlog list                            # Event log
hzn unregister -f                            # Unregister device

# Docker
docker ps | grep edgelake                    # Running containers
docker logs -f <container>                   # View logs
docker volume ls | grep edgelake             # List volumes
```

## Support

- Deployment Plan: [OH_DEPLOYMENT_PLAN.md](OH_DEPLOYMENT_PLAN.md)
- OH Services README: [oh-services/README.md](oh-services/README.md)
- Agent Installation: [oh-services/AGENT_INSTALL.md](oh-services/AGENT_INSTALL.md)
- EdgeLake Documentation: `/path/to/documentation`
