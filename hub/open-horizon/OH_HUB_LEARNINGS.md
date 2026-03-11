# Open Horizon Management Hub - Key Learnings

## Overview

This document captures important learnings from deploying and managing the Open Horizon Management Hub for EdgeLake services.

## Management Hub Architecture

The OH Hub consists of several key components:
- **Exchange**: Service registry and node management (port 3090)
- **AgBot**: Agreement bot that negotiates service deployments
- **CSS (Cloud Sync Service)**: Model Management System (port 9443)
- **PostgreSQL**: Database for Exchange and AgBot
- **Vault (Bao)**: Secrets management

## Organization Structure

Open Horizon uses a hierarchical organization model:
- **System Org (`IBM`)**: The root organization where the agbot is registered
- **User Orgs** (e.g., `myorg`, `trv-services`): Child organizations for services and nodes

### Important: Agbot Serves Multiple Orgs

The agbot registered in the `IBM` system org can serve multiple user organizations. This is the correct architecture:
- **Single agbot** in `IBM` org
- **Multiple user orgs** (each with their own services and nodes)
- Agbot must be explicitly configured to serve each user org

## Adding a New Organization

When adding a new organization to an existing OH Hub deployment:

### 1. Create the Organization in Exchange

```bash
# Set root credentials
export HZN_EXCHANGE_URL=http://<hub-tailscale-ip>:3090/v1

# Create new org (replace YOUR_ORG_NAME and YOUR_ADMIN_PASSWORD)
curl -sS -u "root/root:YOUR_ROOT_PASSWORD" -X POST \
  "$HZN_EXCHANGE_URL/orgs/YOUR_ORG_NAME" \
  -H "Content-Type: application/json" \
  -d '{"label":"Your Org Label","description":"Your org description","heartbeatIntervals":{"minInterval":3,"maxInterval":10,"intervalAdjustment":1}}'

# Create admin user for the new org
curl -sS -u "root/root:YOUR_ROOT_PASSWORD" -X POST \
  "$HZN_EXCHANGE_URL/orgs/YOUR_ORG_NAME/users/admin" \
  -H "Content-Type: application/json" \
  -d '{"password":"YOUR_ADMIN_PASSWORD","admin":true,"email":"admin@your-org.com"}'
```

### 2. Configure AgBot to Serve the New Org

The agbot must be explicitly configured to serve business policies and patterns from the new org:

```bash
# Set IBM org credentials (where agbot is registered)
export HZN_ORG_ID=IBM
export HZN_EXCHANGE_USER_AUTH=IBM/admin:YOUR_IBM_ADMIN_PASSWORD

# Add pattern support for new org (allows pattern-based deployments)
hzn exchange agbot addpattern IBM/agbot IBM "*" YOUR_ORG_NAME
hzn exchange agbot addpattern IBM/agbot YOUR_ORG_NAME "*" YOUR_ORG_NAME

# Add business policy support for new org (CRITICAL for policy-based deployments)
curl -sS -u "root/root:YOUR_ROOT_PASSWORD" -X POST \
  "$HZN_EXCHANGE_URL/orgs/IBM/agbots/agbot/businesspols" \
  -H "Content-Type: application/json" \
  -d '{"businessPolOrgid":"YOUR_ORG_NAME","businessPol":"*","nodeOrgid":"YOUR_ORG_NAME"}'
```

### 3. Verify AgBot Configuration

```bash
# Check agbot is serving the new org (look in logs)
docker logs agbot 2>&1 | grep "serving orgs"
# Should show: [IBM myorg YOUR_ORG_NAME]

# Verify pattern associations
hzn exchange agbot listpattern IBM/agbot

# Verify business policy associations
hzn exchange agbot listbusinesspol IBM/agbot
```

### 4. Publish Services to New Org

```bash
# Set new org credentials
export HZN_ORG_ID=YOUR_ORG_NAME
export HZN_EXCHANGE_USER_AUTH=YOUR_ORG_NAME/admin:YOUR_ADMIN_PASSWORD

# Publish your service (example from our Makefile)
cd /path/to/your/service
make -f Makefile.oh oh-publish-all
```

## Open Horizon Container Management

### Container Naming

Open Horizon creates containers with long names that include the agreement ID:
```
<agreement-id>-<service-name>
```

Example:
```
1e35d6fd4d53da1cf41b6e38353031fa18567c6b201810668b852e33387fd581-service-edgelake-operator
```

This makes it difficult to reference containers. Use filters instead:
```bash
# Find container by image
docker ps --filter "ancestor=<hub-tailscale-ip>:5000/edgelake-mcp"

# Get container ID
CONTAINER_ID=$(docker ps -q --filter "ancestor=<hub-tailscale-ip>:5000/edgelake-mcp")
```

### CLI Access Limitation

Open Horizon containers **do not support TTY** (`docker exec -it` will fail with "input device is not a TTY").

**Solution**: Use `-i` flag only (interactive without TTY):
```bash
# Won't work
docker exec -it $CONTAINER_ID bash

# Works
docker exec -i $CONTAINER_ID bash
```

**Helper Script**: Use `oh-services/edgelake-cli.sh` for easier access:
```bash
./edgelake-cli.sh 'get status'
./edgelake-cli.sh shell
```

## Common Issues and Solutions

### Issue 1: AgBot Not Querying Business Policies

**Symptom**: AgBot serves the org but doesn't query for business policies in that org.

**Cause**: Missing business policy association in the Exchange.

**Solution**: Add business policy association (see step 2 above).

```bash
# Check if association exists
hzn exchange agbot listbusinesspol IBM/agbot

# If missing, add it
curl -sS -u "root/root:YOUR_ROOT_PASSWORD" -X POST \
  "http://<hub-tailscale-ip>:3090/v1/orgs/IBM/agbots/agbot/businesspols" \
  -H "Content-Type: application/json" \
  -d '{"businessPolOrgid":"YOUR_ORG_NAME","businessPol":"*","nodeOrgid":"YOUR_ORG_NAME"}'
```

### Issue 2: Agreements Not Forming

**Checklist**:
1. AgBot is serving the organization
2. Business policy association exists
3. Node policy properties match service policy constraints
4. Service is published and has a deployment/business policy
5. Edge node is registered with correct org

```bash
# Verify on edge device
hzn node list  # Check org
hzn policy list  # Check properties
hzn agreement list  # Check agreements

# Verify in Exchange
hzn exchange node list  # Should see your node
hzn exchange business listpolicy  # Should see deployment policy

# Check agbot logs
docker logs agbot 2>&1 | grep "searching YOUR_ORG_NAME"
```

### Issue 3: Exchange Connection Refused After Restart

**Cause**: PostgreSQL password mismatch after redeployment.

**Solution**: Clean deployment with volume removal.

```bash
# Stop all containers
docker stop exchange-api css-api agbot postgres mongo bao

# Remove containers
docker rm exchange-api css-api agbot postgres mongo bao

# Remove volumes (WARNING: deletes all data)
docker volume rm $(docker volume ls -q | grep -E 'horizon|agbot')

# Redeploy
cd /home/USER/devops/mgmt-hub
./deploy-mgmt-hub.sh
```

### Issue 4: AgBot Config Changes Not Taking Effect

**Symptom**: Changed `/etc/horizon/agbot.json` but agbot still uses old config.

**Cause**: AgBot startup script processes template file with `envsubst`.

**Solution**:
- Edit `/etc/horizon/agbot.json` on host
- Fully restart container: `docker restart agbot`
- Verify config inside container: `docker exec agbot cat /etc/horizon/anax.json | grep ExchangeId`

## AgBot Configuration Details

### Config File Location
- **Host**: `/etc/horizon/agbot.json`
- **Container**: Processed to `/etc/horizon/anax.json`

### Key Configuration Fields
```json
{
  "AgreementBot": {
    "ExchangeId": "IBM/agbot",  // Must match agbot in Exchange
    "ExchangeToken": "...",      // AgBot authentication token
    "ExchangeURL": "http://exchange-api:8080/v1/",
    "NewContractIntervalS": 5,    // How often to search for new agreements
    "ProcessGovernanceIntervalS": 5,  // Agreement governance cycle
    "CheckUpdatedPolicyS": 7,     // Policy update check interval
    "FullRescanS": 600            // Full node rescan interval (10 min)
  }
}
```

### Changing AgBot Organization

**DON'T DO THIS** unless you have a specific reason. The agbot should remain in the `IBM` system org and serve multiple user orgs.

If you must change it:
```bash
# Update config
sudo sed -i 's/"ExchangeId":"OLD_ORG\/agbot"/"ExchangeId":"NEW_ORG\/agbot"/' /etc/horizon/agbot.json

# Register agbot in new org in Exchange
curl -sS -u "root/root:ROOT_PASSWORD" -X PUT \
  "http://localhost:3090/v1/orgs/NEW_ORG/agbots/agbot" \
  -H "Content-Type: application/json" \
  -d '{"token":"AGBOT_TOKEN","name":"EdgeLake AgBot","publicKey":""}'

# Restart
docker restart agbot
```

## Edge Device Registration

### Initial Registration

```bash
# On edge device, configure agent
sudo tee /etc/default/horizon <<EOF
HZN_EXCHANGE_URL=http://<hub-tailscale-ip>:3090/v1
HZN_FSS_CSSURL=http://<hub-tailscale-ip>:9443
EOF

# Restart agent
sudo systemctl restart horizon

# Register with policy
export HZN_ORG_ID=YOUR_ORG_NAME
export HZN_EXCHANGE_USER_AUTH=YOUR_ORG_NAME/admin:YOUR_PASSWORD

hzn register --name=your-node-name --policy=node.policy.json
```

### Node Policy Example

```json
{
  "properties": [
    {"name": "purpose", "value": "edgelake"},
    {"name": "openhorizon.allowPrivileged", "value": true}
  ],
  "constraints": []
}
```

### Monitoring Agreements

```bash
# Watch for agreements (Ctrl-C to exit)
watch -n 2 hzn agreement list

# Check service status
hzn service list

# View event log
hzn eventlog list | tail -50

# Check specific agreement
hzn agreement list <agreement-id>
```

## Useful Commands Reference

### Exchange Queries
```bash
# List organizations
hzn exchange org list

# List services in org
hzn exchange service list YOUR_ORG_NAME/

# List business policies
hzn exchange business listpolicy YOUR_ORG_NAME/

# List nodes in org
hzn exchange node list YOUR_ORG_NAME/

# Get node details
hzn exchange node list YOUR_ORG_NAME/<node-id>
```

### AgBot Management
```bash
# List agbots
hzn exchange agbot list IBM/

# Check agbot patterns
hzn exchange agbot listpattern IBM/agbot

# Check agbot business policies
hzn exchange agbot listbusinesspol IBM/agbot

# View agbot logs
docker logs agbot 2>&1 | tail -100

# Check which orgs agbot serves
docker logs agbot 2>&1 | grep "serving orgs"

# Search for agreement activity
docker logs agbot 2>&1 | grep -i "searching YOUR_ORG"
```

### Troubleshooting Commands
```bash
# Check Exchange connectivity
hzn exchange status

# Check Exchange version
curl -sS http://<hub-tailscale-ip>:3090/v1/admin/version

# View PostgreSQL logs
docker logs postgres 2>&1 | tail -50

# Check container health
docker ps
docker inspect exchange-api | grep -i health
```

## Important Files and Locations

### Management Hub (trv-srv-001)
- Deployment script: `/home/<user>/devops/mgmt-hub/deploy-mgmt-hub.sh`
- Credentials: `/home/<user>/devops/mgmt-hub/my_tokens.txt`
- AgBot config: `/etc/horizon/agbot.json`
- Docker compose: `/home/<user>/devops/mgmt-hub/docker-compose.yml`

### Edge Device
- Agent config: `/etc/default/horizon`
- Node policy: `~/node.policy.json` (user-provided)
- Agent database: `/var/horizon/`

## Credentials Reference

Store credentials securely. From `my_tokens.txt`:

```bash
# Root admin
export HZN_ORG_ID=root
export HZN_EXCHANGE_USER_AUTH=root:YOUR_ROOT_PASSWORD

# IBM org admin (system org)
export HZN_ORG_ID=IBM
export HZN_EXCHANGE_USER_AUTH=IBM/admin:YOUR_IBM_PASSWORD

# User org admin
export HZN_ORG_ID=YOUR_ORG_NAME
export HZN_EXCHANGE_USER_AUTH=YOUR_ORG_NAME/admin:YOUR_ORG_PASSWORD

# AgBot token (stored in agbot.json)
export AGBOT_TOKEN=YOUR_AGBOT_TOKEN
```

## EdgeLake-Specific Deployment Considerations

### IP Address Management

When deploying EdgeLake operators via Open Horizon, special attention is needed for IP address configuration:

#### IP Address Types in EdgeLake
- **`ip`**: External IP address (ISP address for outside connections)
- **`local_ip`**: Internal/overlay IP address for node-to-node communication within the network
- **`OVERLAY_IP`**: Configuration parameter that sets `local_ip` in the blockchain policy

#### Key Issue: Shared External IP
When multiple EdgeLake nodes are deployed behind the same external IP (NAT/ISP router), they will all register with the same `ip` value in the blockchain. This creates a **unique constraint violation** if they use the same ports.

**Example Problem:**
```
Node 1: ip=172.124.189.63, port=32148  (registered)
Node 2: ip=172.124.189.63, port=32148  (CONFLICT!)
```

**Error Message:**
```
New Policy 'operator' Structure Error: None unique IP:Port in policy
Failed to declare !node_type policy on blockchain
```

#### Solution: Unique Ports Per Node

Each operator behind the same external IP must use unique ports:

**Working Configuration:**
```bash
# Operator 1 (docker-compose)
ANYLOG_SERVER_PORT=32148
ANYLOG_REST_PORT=32149
# Result: ip=172.124.189.63:32148, local_ip=<edge-srv-012-tailscale-ip>

# Operator 2 (Open Horizon)
ANYLOG_SERVER_PORT=32248  # Different!
ANYLOG_REST_PORT=32249    # Different!
# Result: ip=172.124.189.63:32248, local_ip=<edge-srv-014-tailscale-ip>
```

**Port Convention:**
- TCP and REST ports always paired: 32X48/32X49
- Master: 32048/32049
- Operator 1: 32148/32149
- Operator 2: 32248/32249
- Query: 32348/32349

### OVERLAY_IP Configuration

The `OVERLAY_IP` parameter is critical for proper network communication in overlay network environments (Tailscale, Nebula, etc.):

**Configuration:**
```bash
# In operator_production.env
OVERLAY_IP=<edge-srv-014-tailscale-ip>  # The Tailscale/overlay IP address
CONN_IP=0.0.0.0           # Bind to all interfaces
```

**What This Does:**
1. EdgeLake auto-detects external IP → sets `ip` field
2. `OVERLAY_IP` is explicitly set → sets `local_ip` field
3. Other nodes use `local_ip` for internal communication
4. External clients use `ip:port` for connections

**Verification:**
```bash
# Check the policy on the master
blockchain get operator where name contains <operator-name>

# Should show:
# "ip": "172.124.189.63"        ← External ISP address
# "local_ip": "<edge-srv-014-tailscale-ip>"   ← Tailscale overlay IP
# "port": 32248
```

### TTY and CLI Access Limitations

Open Horizon containers do not support TTY attachment. This affects EdgeLake CLI access:

**What Doesn't Work:**
```bash
docker attach edgelake-operator  # Hangs, no prompt
docker exec -it <container> bash  # TTY error
```

**Why:**
- OH creates containers without `-t` (TTY) flag
- Only `-i` (stdin) is supported
- EdgeLake detects missing TTY and runs in "Detached Mode"

**Message You'll See:**
```
|=====================================================================================================|
|AnyLog Node in Detached Mode - For Interactive Mode, attach the console to the standard input, output|
|=====================================================================================================|
```

**Solution: Use Helper Script**

A helper script `edgelake-cli.sh` provides CLI access without TTY:

```bash
#!/bin/bash
# Find the EdgeLake container
CONTAINER=$(docker ps --format "{{.ID}}\t{{.Image}}" | grep -i edgelake | head -1 | cut -f1)

if [ "$1" = "shell" ]; then
    docker exec -i $CONTAINER bash
else
    docker exec -i $CONTAINER bash -c "echo '$@' | /app/edgelake_agent"
fi
```

**Usage:**
```bash
# Run EdgeLake commands
~/edgelake-cli.sh "get status"
~/edgelake-cli.sh "get processes"
~/edgelake-cli.sh "blockchain get operator"

# Open bash shell
~/edgelake-cli.sh shell
```

### Blockchain Policy Cleanup

When troubleshooting, you may need to remove stale operator policies from the blockchain:

**Commands:**
```bash
# Local removal (from any node with the policy cached)
blockchain drop policy where id = <policy-id>

# Global removal (from master node)
blockchain delete policy where id = <policy-id> and master = <hub-tailscale-ip>:32048

# Force blockchain sync
blockchain sync
```

**Finding Policy IDs:**
```bash
# List all operators
blockchain get operator

# Find specific operator
blockchain get operator where name contains <operator-name>

# The 'id' field is the policy ID to use for deletion
```

### Container Naming and Agreement Management

Open Horizon creates containers with long agreement-based names:

**Format:** `<agreement-id>-<service-name>`

**Example:** `2875fcad04513167ad84d9869dd26bd935df7ec838db03fc2adcfa7295a90ece-service-edgelake-operator`

**Multiple Agreements Issue:**

When deploying new service versions, Open Horizon may create multiple agreements simultaneously, leading to:
- Multiple containers running (one per version)
- Port conflicts
- Resource contention

**Solution:**
```bash
# List all agreements
hzn agreement list | grep -E "\"name\"|current_agreement_id"

# Cancel old agreements (keep only latest version)
hzn agreement cancel <agreement-id>

# Wait for containers to stop
sleep 10

# Verify single container
docker ps | grep edgelake

# Restart if needed to clear port bindings
docker restart <container-id>
```

### Network Configuration Example

**Complete working configuration for OH-deployed operator:**

```env
# Networking
CONN_IP=0.0.0.0
OVERLAY_IP=<edge-srv-014-tailscale-ip>
ANYLOG_SERVER_PORT=32248
ANYLOG_REST_PORT=32249
ANYLOG_BROKER_PORT=32150
TCP_BIND=false
REST_BIND=false
BROKER_BIND=false

# Blockchain
LEDGER_CONN=<hub-tailscale-ip>:32048
BLOCKCHAIN_SOURCE=master
BLOCKCHAIN_SYNC=30 second

# Node Identity
NODE_TYPE=operator
NODE_NAME=edgelake-operator-oh
COMPANY_NAME=New Company
CLUSTER_NAME=production-cluster
DEFAULT_DBMS=production_data
```

### Troubleshooting Checklist

When deploying EdgeLake via Open Horizon:

1. **Pre-deployment:**
   - [ ] Unique ports assigned (not conflicting with other nodes behind same external IP)
   - [ ] OVERLAY_IP set to correct Tailscale/overlay address
   - [ ] Service definition includes all required userInput parameters
   - [ ] Helper script deployed to edge device for CLI access

2. **Post-deployment:**
   - [ ] Agreement formed successfully
   - [ ] Only one container running (cancel old agreements)
   - [ ] Container restarted to clear port bindings
   - [ ] Processes running: TCP, REST, Operator, Blockchain Sync
   - [ ] Node visible in master's `test network`
   - [ ] Policy shows correct `local_ip` in blockchain

3. **Verification:**
   ```bash
   # On master node
   test network
   # Should show: <edge-srv-014-tailscale-ip>:32248 | operator | <node-name> | +

   # On operator node
   ~/edgelake-cli.sh "get processes"
   # TCP should show: Listening on: <external-ip>:32248

   # Remote command execution - verify from any node
   run client (<edge-srv-014-tailscale-ip>:32248) get status
   # Should return: edgelake-operator-oh_... running

   # Other useful remote commands
   run client (<edge-srv-014-tailscale-ip>:32248) get processes
   run client (<edge-srv-014-tailscale-ip>:32248) blockchain get operator
   ```

### Remote Command Execution

EdgeLake allows you to execute commands on remote nodes from any node in the network using the `run client` syntax:

**Syntax:**
```
run client (<ip>:<port>) <command>
```

**Examples:**
```bash
# Check status of OH operator from master
run client (<edge-srv-014-tailscale-ip>:32248) get status

# Get processes on remote node
run client (<edge-srv-014-tailscale-ip>:32248) get processes

# Query blockchain on remote node
run client (<edge-srv-014-tailscale-ip>:32248) blockchain get operator

# Check disk usage on remote node
run client (<edge-srv-014-tailscale-ip>:32248) get disk usage
```

**Response Format:**
```
[From Node <edge-srv-014-tailscale-ip>:32248]

'edgelake-operator-oh_a42e03c963cad5081cb576cbc5d2b9f324565fcc@172.124.189.63:32248 running'
```

This is particularly useful for:
- Verifying OH-deployed operators without SSH access
- Monitoring remote nodes from a central location
- Troubleshooting connectivity issues
- Validating blockchain synchronization across nodes

## Next Steps: Kubernetes Cluster Deployment

### Upcoming Deployment: OH + Kubernetes + Helm

**Target Environment:**
- **Host**: 100.74.102.38
- **Platform**: Kubernetes cluster with Open Horizon agent
- **Deployment Method**: Helm chart + Open Horizon service orchestration
- **Tailscale**: Overlay IP will be outside the cluster (on the host)

**Key Considerations to Address:**
1. **Network Architecture**:
   - Tailscale runs on host (100.74.102.38), not inside K8s pods
   - Need ingress controller to route traffic from Tailscale IP to K8s service
   - Determine ingress type (NodePort, LoadBalancer, or Ingress resource)

2. **OVERLAY_IP Configuration**:
   - Set to host's Tailscale IP (100.74.102.38)
   - Configure K8s service to expose on this IP
   - Ensure EdgeLake advertises correct `local_ip` in blockchain

3. **Port Mapping**:
   - Choose unique ports (32X48/32X49 pattern)
   - Map K8s service ports to EdgeLake container ports
   - Ensure no conflicts with existing operators

4. **Open Horizon Integration**:
   - OH agent runs on K8s host
   - Service definition must include K8s/Helm deployment specs
   - Agreement bot must be able to manage K8s workloads

5. **Helm Chart Adaptation**:
   - Review existing Helm chart in repository
   - Adapt for OH service definition format
   - Ensure compatibility with OH container orchestration

**Reference Files:**
- Helm chart: `/path/to/trv-edgelake-infra/helm/` (recently added)
- Working OH service: `oh-services/operator/service.definition.json` (v1.4.3)
- Working config: `oh-services/operator/configurations/operator_production.env`

**Questions to Resolve:**
- [ ] Which ingress controller/method to use?
- [ ] How does OH deploy to K8s (direct kubectl, helm, operator)?
- [ ] Does OH support Helm chart deployments natively?
- [ ] How to handle persistent volumes in K8s via OH?
- [ ] Service mesh considerations for internal communication?

## Best Practices

1. **Always use the IBM agbot** - Don't create multiple agbots or move it to user orgs
2. **Add orgs to existing agbot** - Configure the IBM agbot to serve new orgs
3. **Test connectivity first** - Use `hzn exchange status` before registration
4. **Monitor agbot logs** - Check that new org appears in "serving orgs" after configuration
5. **Verify both patterns AND business policies** - Both associations are needed for full functionality
6. **Wait for full rescan** - AgBot rescans every 10 minutes (FullRescanS: 600)
7. **Check node policy matches service constraints** - Policy evaluation requires exact property matches

## Timeline for Agreement Formation

After configuring agbot to serve a new org:
1. **Immediate**: AgBot sees new org in Exchange changes (~5 seconds)
2. **Immediate**: AgBot starts querying for patterns and business policies
3. **Within 10 seconds**: AgBot discovers business policies
4. **Within 5-15 seconds**: AgBot searches for compatible nodes
5. **Within 30 seconds**: Agreement proposal sent to node
6. **Within 1-2 minutes**: Agreement finalized, service deployed

If no agreement after 5 minutes, troubleshoot using commands above.

## Related Documentation

- EdgeLake Documentation: `/path/to/documentation`
- Open Horizon Docs: https://open-horizon.github.io/
- Our Deployment Plan: `OH_DEPLOYMENT_PLAN.md`
- Quick Start: `QUICKSTART_OH.md`
