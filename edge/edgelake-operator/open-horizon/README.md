# Open Horizon EdgeLake Services

This directory contains Open Horizon service definitions for deploying EdgeLake nodes via policy-based deployment.

## Directory Structure

```
oh-services/
├── operator/
│   ├── configurations/
│   │   └── operator_production.env          # EdgeLake operator configuration
│   ├── service.definition.json.template     # OH service definition template
│   ├── service.policy.json                  # Service policy (constraints)
│   ├── service.deployment.json              # Generated deployment policy
│   ├── node.policy.json                     # Node policy for edge devices
│   └── create_deployment_policy.py          # Script to generate deployment policy
└── README.md
```

## Quick Start

### 1. Configure Environment

Edit the root `Makefile.oh` or set environment variables:

```bash
export HZN_ORG_ID=myorg
export HZN_EXCHANGE_USER_AUTH=admin:<your-password>
export HZN_EXCHANGE_URL=http://<hub-tailscale-ip>:3090/v1
export SERVICE_VERSION=1.3.5
export DOCKER_IMAGE_BASE=<hub-tailscale-ip>:5000/edgelake-mcp
export DOCKER_IMAGE_VERSION=amd64-latest
```

### 2. Customize Operator Configuration

Edit `operator/configurations/operator_production.env`:

- Set `NODE_NAME` to unique name for this deployment
- Update `LEDGER_CONN` to point to your master node (default: <hub-tailscale-ip>:32048)
- Configure `CLUSTER_NAME` and `DEFAULT_DBMS`
- Adjust other settings as needed

### 3. Publish Service to OH Exchange

```bash
# From trv-edgelake-infra directory
make -f Makefile.oh oh-publish-all
```

This will:
1. Generate service definition from template
2. Generate deployment policy from configuration
3. Publish service to Exchange
4. Publish service policy
5. Publish deployment policy

### 4. Register Edge Device

On the edge device (e.g., <edge-srv-014-tailscale-ip>):

```bash
# Install OH agent (see AGENT_INSTALL.md)

# Register device with node policy
hzn register \
  --org=myorg \
  --user-pw=admin:<password> \
  --name=edgelake-operator-device \
  --policy=oh-services/operator/node.policy.json

# Monitor agreement formation
watch hzn agreement list
```

## Configuration Files

### operator_production.env

Main configuration file for EdgeLake operator. Key settings:

- **NODE_TYPE**: Must be "operator"
- **NODE_NAME**: Unique identifier for this operator instance
- **LEDGER_CONN**: IP:port of master node (<hub-tailscale-ip>:32048)
- **ANYLOG_SERVER_PORT**: TCP port for node communication (default: 32148)
- **ANYLOG_REST_PORT**: REST API port (default: 32149)
- **CLUSTER_NAME**: Operator cluster name
- **DEFAULT_DBMS**: Default logical database name
- **DB_TYPE**: sqlite or psql

### service.definition.json.template

Defines the EdgeLake container deployment:
- Docker image and version
- Network mode (host)
- Volume mounts
- User input parameters

Variables are substituted at build time:
- `$SERVICE_NAME`
- `$SERVICE_VERSION`
- `$DOCKER_IMAGE_BASE`
- `$DOCKER_IMAGE_VERSION`
- `$ARCH`

### service.policy.json

Service-level policy defining constraints:
```json
{
  "constraints": [
    "openhorizon.allowPrivileged == true AND purpose == edgelake"
  ]
}
```

Devices must have:
- `openhorizon.allowPrivileged = true` (required for privileged containers)
- `purpose = edgelake`

### service.deployment.json

Generated from `operator_production.env` using `create_deployment_policy.py`.

Contains:
- Service version and org
- Deployment constraints (match service policy)
- User input values from configuration file

### node.policy.json

Policy template for edge devices:
```json
{
  "properties": [
    {"name": "purpose", "value": "edgelake"},
    {"name": "openhorizon.allowPrivileged", "value": true}
  ]
}
```

Devices register with this policy to match deployment policy constraints.

## Makefile Commands

See `Makefile.oh` for all available commands. Key ones:

```bash
# Publishing
make -f Makefile.oh oh-publish-all              # Publish everything
make -f Makefile.oh oh-publish-service          # Publish service only
make -f Makefile.oh oh-publish-deployment       # Publish deployment policy only

# Inspection
make -f Makefile.oh oh-list-services            # List published services
make -f Makefile.oh oh-list-deployments         # List deployment policies
make -f Makefile.oh oh-show-service             # Show service details
make -f Makefile.oh oh-show-deployment          # Show deployment details

# Cleanup
make -f Makefile.oh oh-remove-service           # Remove service from Exchange
make -f Makefile.oh oh-remove-deployment        # Remove deployment policy
make -f Makefile.oh oh-clean                    # Clean local generated files

# Validation
make -f Makefile.oh oh-check-vars               # Validate configuration
make -f Makefile.oh oh-test-connection          # Test Exchange connection
```

## Workflow

### Initial Setup

1. **Hub Side (<hub-tailscale-ip>)**:
   ```bash
   cd /path/to/trv-edgelake-infra

   # Validate configuration
   make -f Makefile.oh oh-check-vars

   # Test Exchange connection
   make -f Makefile.oh oh-test-connection

   # Publish service
   make -f Makefile.oh oh-publish-all
   ```

2. **Edge Device Side (<edge-srv-014-tailscale-ip>)**:
   ```bash
   # Install OH agent (see AGENT_INSTALL.md)

   # Configure agent
   export HZN_ORG_ID=myorg
   export HZN_EXCHANGE_URL=http://<hub-tailscale-ip>:3090/v1
   export HZN_FSS_CSSURL=http://<hub-tailscale-ip>:9443

   # Copy node policy to device
   scp oh-services/operator/node.policy.json <user>@<edge-srv-014-tailscale-ip>:~/

   # Register device
   hzn register --name=operator-prod-1 --policy=~/node.policy.json

   # Monitor
   watch hzn agreement list
   docker ps | grep edgelake
   ```

### Updating Service

When you need to update the configuration or version:

```bash
# Update configuration
vi oh-services/operator/configurations/operator_production.env

# Increment version
export SERVICE_VERSION=1.3.6

# Republish
make -f Makefile.oh oh-publish-all

# Devices will auto-update based on policy
```

### Debugging

**Service not appearing:**
```bash
# Check if service is published
make -f Makefile.oh oh-list-services

# Check deployment policy
make -f Makefile.oh oh-show-deployment
```

**Device not getting agreement:**
```bash
# On device:
hzn eventlog list
hzn policy list
hzn exchange node list

# On hub:
hzn exchange service list --org=$HZN_ORG_ID
hzn exchange deployment listpolicy --org=$HZN_ORG_ID
```

**Container not starting:**
```bash
# On device:
docker ps -a | grep edgelake
docker logs <container-name>
hzn service log <service-name>
```

## Notes

- All containers run in `network_mode: host` for direct access to ports
- Containers require `privileged: true` mode
- Volumes are prefixed with `edgelake-operator-`
- Default ports: 32148 (TCP), 32149 (REST), 32150 (MQTT broker)
- Master node connection: <hub-tailscale-ip>:32048

## References

- [OH_DEPLOYMENT_PLAN.md](../OH_DEPLOYMENT_PLAN.md) - Full deployment plan
- [AGENT_INSTALL.md](AGENT_INSTALL.md) - Agent installation guide
- EdgeLake documentation: `/path/to/documentation`
