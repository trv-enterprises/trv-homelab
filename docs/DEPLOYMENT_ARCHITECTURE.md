# EdgeLake Deployment Architecture: Docker Compose to Node Configuration

## Overview

This document explains the complete flow from running `make up` to a fully configured EdgeLake node, including how environment variables flow through the system and how nodes discover each other via the Master node.

## Table of Contents

1. [System Components](#system-components)
2. [The Complete Flow](#the-complete-flow)
3. [Environment Variable Flow](#environment-variable-flow)
4. [Policy Creation and Publishing](#policy-creation-and-publishing)
5. [Node Discovery and Network Formation](#node-discovery-and-network-formation)
6. [Tailscale Integration](#tailscale-integration)
7. [Configuration Reference](#configuration-reference)

---

## System Components

### 1. Docker Compose Repository (`docker-compose/`)

**Purpose**: Container orchestration and environment configuration

**Structure**:
```
docker-compose/
├── Makefile                              # Main orchestration (make up, make down, etc.)
├── docker-makefiles/
│   ├── .env                              # Global Docker settings (IMAGE, INIT_TYPE)
│   ├── docker-compose-template-base.yaml # Container definition template
│   ├── master-configs/
│   │   ├── base_configs.env              # Core settings (NODE_TYPE, ports, DB, LEDGER_CONN)
│   │   └── advance_configs.env           # Advanced settings (OVERLAY_IP, geolocation, etc.)
│   ├── operator-configs/
│   │   ├── base_configs.env
│   │   └── advance_configs.env
│   └── query-configs/
│       ├── base_configs.env
│       └── advance_configs.env
```

**Responsibilities**:
- Define container configuration (image, ports, volumes)
- Load environment variables from `.env` files
- Start/stop EdgeLake containers
- Mount volumes for persistent data

### 2. Deployment Scripts (`deployment-scripts/`)

**Purpose**: Node initialization and configuration logic

**Structure**:
```
deployment-scripts/
└── node-deployment/
    ├── main.al                           # Entry point (called by Dockerfile ENTRYPOINT)
    ├── set_params.al                     # Converts $ENV_VARS → !local_vars
    ├── connect_blockchain.al             # Blockchain/Master connection setup
    └── policies/
        ├── config_policy.al              # Creates config policy from !local_vars
        ├── node_policy.al                # Creates node registration policy
        └── publish_policy.al             # Publishes policies to blockchain/Master
```

**Responsibilities**:
- Read environment variables from container
- Convert env vars to EdgeLake variables
- Build policy JSON structures
- Connect to blockchain/Master node
- Register node in the network
- Start EdgeLake services (TCP, REST, MCP, etc.)

### 3. EdgeLake Core (`EdgeLake/edge_lake/`)

**Purpose**: The EdgeLake runtime and command processor

**Key Components**:
- `edgelake.py`: Main entry point
- `cmd/member_cmd.py`: Command processor (executes `.al` scripts)
- `tcpip/`: Networking layer (TCP, REST, HTTP servers)
- `blockchain/`: Metadata and policy management
- `dbms/`: Database adapters (SQLite, PostgreSQL, MongoDB)

---

## The Complete Flow

### Phase 1: Container Startup (Docker Compose)

```bash
cd docker-compose
make up master    # Or operator, or query
```

**What happens**:

1. **Makefile Processing** (`docker-compose/Makefile`):
   ```makefile
   # Line 77-88: Reads env files and exports variables
   export NODE_NAME := $(shell cat docker-makefiles/master-configs/base_configs.env | grep "NODE_NAME=" ...)
   export NODE_TYPE := $(shell cat docker-makefiles/master-configs/base_configs.env | grep "NODE_TYPE=" ...)
   export ANYLOG_SERVER_PORT := ...
   export OVERLAY_IP := $(shell cat docker-makefiles/master-configs/advance_configs.env | grep "OVERLAY_IP=" ...)
   ```

2. **Docker Compose Template Rendering**:
   ```yaml
   # docker-compose-template-base.yaml
   services:
     ${NODE_NAME}:                        # e.g., "edgelake-master"
       image: ${IMAGE}:${TAG}             # e.g., "edgelake-mcp:latest"
       env_file:
         - docker-makefiles/master-configs/base_configs.env
         - docker-makefiles/master-configs/advance_configs.env
         - docker-makefiles/.env
       network_mode: host                 # Uses host networking (direct access to ports)
       volumes:
         - ${NODE_NAME}-anylog:/app/EdgeLake/anylog
         - ${NODE_NAME}-blockchain:/app/EdgeLake/blockchain
         - ${NODE_NAME}-data:/app/EdgeLake/data
         - ${NODE_NAME}-local-scripts:/app/deployment-scripts
   ```

3. **Container Launch**:
   - Docker creates container with name `${NODE_NAME}` (e.g., `edgelake-master`)
   - All variables from `.env` files are injected as **environment variables**
   - Container starts with `ENTRYPOINT` from Dockerfile

### Phase 2: EdgeLake Initialization (Dockerfile ENTRYPOINT)

**Dockerfile** (`EdgeLake/Dockerfile`):
```dockerfile
# Line 46: Container entry point
ENTRYPOINT python3 /app/EdgeLake/edge_lake/edgelake.py process /app/deployment-scripts/node-deployment/main.al
```

**Translation**: When container starts, EdgeLake immediately runs `main.al` script

### Phase 3: Deployment Script Execution

**Step 1: `main.al` Entry Point** (`deployment-scripts/node-deployment/main.al`):

```al
# Line 44-49: Set up directory paths
set anylog_path = /app
if $ANYLOG_PATH then set anylog_path = $ANYLOG_PATH
set anylog home !anylog_path
create work directories

# Line 61: Load environment variables
process !local_scripts/set_params.al

# Line 69: Create and publish configuration policy
process !local_scripts/policies/config_policy.al
```

**Step 2: `set_params.al` - Environment Variable Conversion** (`deployment-scripts/node-deployment/set_params.al`):

This script converts **shell environment variables** (`$VARIABLE`) into **EdgeLake variables** (`!variable`):

```al
# Line 28-40: Read env vars and convert to local vars
if $NODE_TYPE then set node_type = $NODE_TYPE
if $NODE_NAME then node_name = $NODE_NAME
if $COMPANY_NAME then company_name = $COMPANY_NAME
if $LEDGER_CONN then ledger_conn = $LEDGER_CONN

# Line 106: Read Tailscale/overlay IP
if $OVERLAY_IP then overlay_ip = $OVERLAY_IP

# Line 75-86: Read networking ports
if $ANYLOG_SERVER_PORT then anylog_server_port = $ANYLOG_SERVER_PORT
if $ANYLOG_REST_PORT then anylog_rest_port = $ANYLOG_REST_PORT
```

**Key Concept**: `$VARIABLE` = environment variable (from Docker), `!variable` = EdgeLake script variable

**Step 3: `config_policy.al` - Policy Creation** (`deployment-scripts/node-deployment/policies/config_policy.al`):

Builds a **config policy** JSON structure using the converted variables:

```al
# Line 50-53: Initialize policy structure
new_policy = ""
set policy new_policy [config] = {}
set policy new_policy [config][name] = !config_name
set policy new_policy [config][company] = !company_name
set policy new_policy [config][node_type] = !node_type

# Line 61-63: Set IP addresses (uses OVERLAY_IP if provided)
set policy new_policy [config][ip] = '!external_ip'
set policy new_policy [config][local_ip] = '!ip'
if !overlay_ip then set policy new_policy [config][local_ip] = '!overlay_ip'

# Line 65-66: Set ports
set policy new_policy [config][port] = '!anylog_server_port.int'
set policy new_policy [config][rest_port] = '!anylog_rest_port.int'

# Line 107-117: Define startup scripts (for Master/Query nodes)
set policy new_policy [config][script] = [
    "process !local_scripts/database/deploy_database.al",
    "if !blockchain_source == master then blockchain seed from !ledger_conn",
    "process !local_scripts/connect_blockchain.al",
    "process !local_scripts/policies/node_policy.al",
    "run scheduler 1",
    "process !anylog_path/EdgeLake/edge_lake/mcp_server/autostart.al",
    ...
]
```

**Resulting Policy** (example for Master node):
```json
{
  "config": {
    "name": "master-new-company-configs",
    "company": "New Company",
    "node_type": "master",
    "ip": "23.45.67.89",              // External IP (auto-detected)
    "local_ip": "100.64.1.10",       // Tailscale IP (from $OVERLAY_IP)
    "port": 32048,
    "rest_port": 32049,
    "threads": 6,
    "rest_threads": 6,
    "rest_timeout": 30,
    "script": [
      "process !local_scripts/database/deploy_database.al",
      "blockchain seed from 127.0.0.1:32048",
      "process !local_scripts/connect_blockchain.al",
      "process !local_scripts/policies/node_policy.al",
      "run scheduler 1",
      ...
    ]
  }
}
```

**Step 4: `publish_policy.al` - Policy Publication** (`deployment-scripts/node-deployment/policies/publish_policy.al`):

```al
# Line 31: Prepare policy for blockchain
blockchain prepare policy !new_policy

# Line 33-34: Insert policy into blockchain/Master
if !is_config == true then blockchain insert where policy=!new_policy and local=true
else blockchain insert where policy=!new_policy and local=true and master=!ledger_conn
```

**What happens**:
- Policy is signed (if authentication enabled)
- Policy is validated (JSON format check)
- Policy is published to Master node (via `LEDGER_CONN`)
- Policy is stored locally in `blockchain/` directory

**Step 5: Policy Execution** - Config Policy Script Runs

After publishing the config policy, EdgeLake executes the `script` array from the policy:

```al
# Line 177: Execute the config policy
config from policy where id = !config_id
```

This triggers the scripts defined in the config policy, which includes:

1. **Database Setup** (`deploy_database.al`)
2. **Blockchain Connection** (`connect_blockchain.al`)
3. **Node Registration** (`node_policy.al`) ← Creates the **node policy**
4. **Service Startup** (TCP server, REST server, MCP server, etc.)

### Phase 4: Node Registration

**`node_policy.al`** (`deployment-scripts/node-deployment/policies/node_policy.al`):

Creates a **node policy** that registers this specific node instance:

```al
# Line 52-54: Initialize node policy
set policy new_policy [!node_type] = {}
set policy new_policy [!node_type][name] = !node_name
set policy new_policy [!node_type][company] = !company_name

# Line 61-65: Network configuration (IP selection logic)
set policy new_policy [!node_type][ip] = !external_ip
if !tcp_bind == true and !overlay_ip then
    set policy new_policy [!node_type][ip] = !overlay_ip
if !tcp_bind == false and !overlay_ip then
    set policy new_policy [!node_type][local_ip] = !overlay_ip

# Line 67-68: Ports
set policy new_policy [!node_type][port] = !anylog_server_port.int
set policy new_policy [!node_type][rest_port] = !anylog_rest_port.int

# Line 76: Operator nodes also include cluster ID
if !node_type == operator then
    set policy new_policy [operator][cluster] = !cluster_id
```

**Resulting Node Policy** (example for Operator):
```json
{
  "operator": {
    "name": "edgelake-operator",
    "company": "New Company",
    "hostname": "operator-host-1",
    "ip": "100.64.1.20",           // Tailscale IP (from OVERLAY_IP)
    "port": 32148,
    "rest_port": 32149,
    "cluster": "abc123...",         // Cluster policy ID
    "loc": "37.425,-122.078",
    "country": "US",
    "state": "CA",
    "city": "Mountain View"
  }
}
```

This node policy is then published to the Master node using the same `publish_policy.al` flow.

---

## Environment Variable Flow

### Complete Trace

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. SHELL ENVIRONMENT (docker-compose host)                      │
│    make up master                                               │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 2. MAKEFILE (docker-compose/Makefile)                           │
│    Reads: docker-makefiles/master-configs/base_configs.env      │
│    Exports: NODE_TYPE=master, OVERLAY_IP=100.64.1.10, etc.      │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 3. DOCKER COMPOSE (docker-compose-template-base.yaml)           │
│    env_file:                                                    │
│      - master-configs/base_configs.env                          │
│      - master-configs/advance_configs.env                       │
│    Injects as environment variables into container              │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 4. CONTAINER ENVIRONMENT (inside Docker container)              │
│    $NODE_TYPE = "master"                                        │
│    $OVERLAY_IP = "100.64.1.10"                                  │
│    $LEDGER_CONN = "127.0.0.1:32048"                             │
│    $ANYLOG_SERVER_PORT = "32048"                                │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 5. DOCKERFILE ENTRYPOINT                                        │
│    python3 edgelake.py process main.al                          │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 6. MAIN.AL (deployment-scripts/node-deployment/main.al)         │
│    process !local_scripts/set_params.al                         │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 7. SET_PARAMS.AL (converts $ENV → !local_var)                   │
│    if $NODE_TYPE then node_type = $NODE_TYPE                    │
│    if $OVERLAY_IP then overlay_ip = $OVERLAY_IP                 │
│    if $LEDGER_CONN then ledger_conn = $LEDGER_CONN              │
│                                                                 │
│    Result:                                                      │
│      !node_type = "master"                                      │
│      !overlay_ip = "100.64.1.10"                                │
│      !ledger_conn = "127.0.0.1:32048"                           │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 8. CONFIG_POLICY.AL (builds policy JSON)                        │
│    set policy new_policy [config][ip] = '!external_ip'          │
│    if !overlay_ip then                                          │
│      set policy new_policy [config][local_ip] = '!overlay_ip'   │
│    set policy new_policy [config][port] = '!anylog_server_port' │
│                                                                 │
│    Result: JSON policy with all configuration                   │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 9. PUBLISH_POLICY.AL (publish to blockchain/Master)             │
│    blockchain insert where policy=!new_policy and               │
│                      master=!ledger_conn                        │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 10. MASTER NODE (receives and stores policy)                    │
│     - Validates policy                                          │
│     - Stores in blockchain/ directory                           │
│     - Makes available to all nodes via metadata sync            │
└─────────────────────────────────────────────────────────────────┘
```

### Variable Syntax Reference

| Syntax | Type | Scope | Example |
|--------|------|-------|---------|
| `$VARIABLE` | Shell environment variable | Container environment | `$NODE_TYPE`, `$OVERLAY_IP` |
| `!variable` | EdgeLake script variable | Current `.al` script | `!node_type`, `!overlay_ip` |
| `!variable.int` | Type conversion | Converts to integer | `!anylog_server_port.int` |
| `!variable.name` | String sanitization | Removes spaces, lowercase | `!company_name.name` |
| `[*][field]` | JSON path query | Extracts field from JSON | `blockchain get config ... bring [*][id]` |

---

## Policy Creation and Publishing

### Policy Types

EdgeLake uses two main policy types during deployment:

#### 1. Config Policy

**Purpose**: Defines the **configuration and startup sequence** for a node

**Structure**:
```json
{
  "config": {
    "name": "string",          // Unique config name
    "company": "string",       // Company identifier
    "node_type": "string",     // master|operator|query|generic
    "ip": "string",            // External/public IP
    "local_ip": "string",      // Internal/Tailscale IP
    "port": integer,           // TCP server port
    "rest_port": integer,      // REST API port
    "broker_port": integer,    // MQTT broker port (optional)
    "threads": integer,        // TCP worker threads
    "rest_threads": integer,   // REST worker threads
    "rest_timeout": integer,   // REST timeout in seconds
    "script": [                // Startup commands (executed in order)
      "command1",
      "command2",
      ...
    ]
  }
}
```

**Lifecycle**:
1. Created in `config_policy.al`
2. Published to Master node
3. Retrieved by node on startup
4. Executed via `config from policy where id = !config_id`

#### 2. Node Policy

**Purpose**: **Registers** a node instance in the network (for node discovery)

**Structure** (Operator example):
```json
{
  "operator": {
    "name": "string",          // Node instance name
    "company": "string",       // Company identifier
    "hostname": "string",      // Container hostname
    "ip": "string",            // Node IP (Tailscale if OVERLAY_IP set)
    "local_ip": "string",      // Local IP (optional)
    "port": integer,           // TCP server port
    "rest_port": integer,      // REST API port
    "cluster": "string",       // Cluster policy ID (operators only)
    "member": integer,         // Operator member ID (optional)
    "loc": "string",           // GPS coordinates (lat,lon)
    "country": "string",       // Country code
    "state": "string",         // State/region
    "city": "string"           // City
  }
}
```

**Lifecycle**:
1. Created in `node_policy.al`
2. Published to Master node
3. Synced to all nodes via `blockchain sync`
4. Used by Query nodes to route queries to Operators

### Policy Publishing Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ Node (Operator/Query)                                           │
│                                                                 │
│  1. set_params.al                                               │
│     ↓ Converts $ENV → !vars                                     │
│                                                                 │
│  2. config_policy.al                                            │
│     ↓ Builds config policy JSON                                 │
│                                                                 │
│  3. publish_policy.al                                           │
│     ↓ blockchain insert where master=!ledger_conn               │
│     ↓ (sends HTTP POST to Master REST API)                      │
└─────────────────────────────────────────────────────────────────┘
                            ↓ HTTP POST
                            ↓ Policy JSON
┌─────────────────────────────────────────────────────────────────┐
│ Master Node (100.64.1.10:32048)                                 │
│                                                                 │
│  1. Receives policy via REST API                                │
│  2. Validates JSON structure                                    │
│  3. Stores in blockchain/ directory                             │
│  4. Broadcasts to other nodes (if applicable)                   │
│  5. Makes available via metadata queries                        │
└─────────────────────────────────────────────────────────────────┘
                            ↓ Metadata sync
                            ↓ (periodic: every 30 seconds)
┌─────────────────────────────────────────────────────────────────┐
│ All Nodes in Network                                            │
│                                                                 │
│  blockchain sync where source=master and time=30 second         │
│                       and connection=!ledger_conn               │
│                                                                 │
│  → Downloads new policies from Master                           │
│  → Updates local blockchain/ directory                          │
│  → Can now query: blockchain get operator where ...             │
└─────────────────────────────────────────────────────────────────┘
```

### Key Commands

| Command | Purpose |
|---------|---------|
| `blockchain prepare policy !policy` | Validates policy JSON structure |
| `blockchain insert where policy=!policy and master=!ledger_conn` | Publishes policy to Master node |
| `blockchain sync where connection=!ledger_conn` | Downloads policies from Master |
| `blockchain get operator where ...` | Queries node policies |
| `config from policy where id=!config_id` | Executes a config policy |

---

## Node Discovery and Network Formation

### How Nodes Find Each Other

EdgeLake uses a **metadata-driven discovery** model where all nodes register with the Master node, and other nodes query the Master for peer locations.

### Step-by-Step Discovery Process

#### 1. Master Node Startup

```
Master Node (100.64.1.10:32048)
├── Starts TCP server on port 32048
├── Starts REST server on port 32049
├── Creates blockchain seed (local metadata store)
├── Publishes own config and node policies
└── Waits for other nodes to connect
```

Configuration (Master's `.env` files):
```bash
# base_configs.env
NODE_TYPE=master
ANYLOG_SERVER_PORT=32048
ANYLOG_REST_PORT=32049
LEDGER_CONN=127.0.0.1:32048    # Points to itself

# advance_configs.env
OVERLAY_IP=100.64.1.10          # Tailscale IP
```

#### 2. Operator Node Startup

```
Operator Node (100.64.1.20:32148)
├── Starts TCP server on port 32148
├── Starts REST server on port 32149
├── Connects to Master (via LEDGER_CONN)
│   └── blockchain seed from 100.64.1.10:32048
├── Publishes node policy to Master
│   └── {"operator": {"ip": "100.64.1.20", "port": 32148, ...}}
├── Starts periodic metadata sync (every 30s)
│   └── blockchain sync where connection=100.64.1.10:32048
└── Starts Operator service (accepts data ingestion)
```

Configuration (Operator's `.env` files):
```bash
# base_configs.env
NODE_TYPE=operator
ANYLOG_SERVER_PORT=32148
ANYLOG_REST_PORT=32149
LEDGER_CONN=100.64.1.10:32048   # Points to Master's Tailscale IP

# advance_configs.env
OVERLAY_IP=100.64.1.20          # Operator's Tailscale IP
```

#### 3. Query Node Startup

```
Query Node (100.64.1.30:32348)
├── Starts TCP server on port 32348
├── Starts REST server on port 32349
├── Connects to Master (via LEDGER_CONN)
│   └── blockchain seed from 100.64.1.10:32048
├── Publishes node policy to Master
│   └── {"query": {"ip": "100.64.1.30", "port": 32348, ...}}
├── Starts periodic metadata sync (every 30s)
│   └── Downloads all operator/cluster policies
└── Ready to route queries
```

Configuration (Query's `.env` files):
```bash
# base_configs.env
NODE_TYPE=query
ANYLOG_SERVER_PORT=32348
ANYLOG_REST_PORT=32349
LEDGER_CONN=100.64.1.10:32048   # Points to Master's Tailscale IP

# advance_configs.env
OVERLAY_IP=100.64.1.30          # Query's Tailscale IP
```

#### 4. Query Routing (How Query Finds Operators)

When a user sends a SQL query to the Query node:

```sql
-- User query
SELECT device_name, timestamp, value
FROM sensor_data
WHERE timestamp > now() - 1 hour
```

**Query Node Processing**:

```
1. Query Received (via REST API)
   ↓
2. Consult Metadata (blockchain get operator where ...)
   ↓ Finds: operator at 100.64.1.20:32148
   ↓
3. Send Query to Operator
   ↓ TCP request to 100.64.1.20:32148
   ↓
4. Operator Executes Query (local SQLite/PostgreSQL)
   ↓ Returns results
   ↓
5. Query Node Aggregates Results
   ↓ (if multiple operators, combines results)
   ↓
6. Return to User
   ↓ JSON response via REST API
```

**Metadata Query** (internal):
```al
# Query node finds all operators for a cluster
operator_ips = blockchain get operator where cluster=!cluster_id bring [operator][ip]
operator_ports = blockchain get operator where cluster=!cluster_id bring [operator][port]

# Result: List of operator endpoints
# ["100.64.1.20:32148", "100.64.1.21:32148", ...]
```

### Network Topology

```
                    ┌────────────────────────────┐
                    │   Master Node              │
                    │   100.64.1.10:32048        │
                    │                            │
                    │  - Metadata repository     │
                    │  - Policy storage          │
                    │  - Blockchain seed         │
                    └────────────────────────────┘
                              ↑  ↑
                 LEDGER_CONN  │  │  LEDGER_CONN
                  (metadata   │  │   (metadata
                    sync)     │  │     sync)
                              │  │
        ┌─────────────────────┘  └─────────────────────┐
        │                                                │
        ↓                                                ↓
┌────────────────────────────┐         ┌────────────────────────────┐
│ Operator Node #1           │         │ Query Node                 │
│ 100.64.1.20:32148          │←────────│ 100.64.1.30:32348          │
│                            │  Query  │                            │
│ - Data ingestion (MQTT)    │  Routing│ - Query orchestration      │
│ - Local DBMS (SQLite)      │         │ - Query aggregation        │
│ - Cluster member           │         │ - REST API endpoint        │
└────────────────────────────┘         └────────────────────────────┘
        ↑
        │ Queries node registry
        │ via Master metadata
        │
┌────────────────────────────┐
│ Operator Node #2           │
│ 100.64.1.21:32148          │
│                            │
│ - Same cluster as #1       │
│ - HA/load balancing        │
└────────────────────────────┘
```

**Key Relationships**:
- **All nodes** → **Master** (via `LEDGER_CONN`) for metadata sync
- **Query** → **Operators** (discovered via Master metadata) for query execution
- **Operators** → **Operators** (optional, for HA replication)

---

## Tailscale Integration

### Why Tailscale?

Tailscale creates a **private mesh network** using WireGuard, allowing nodes to communicate securely even if they're behind NATs or firewalls. Perfect for distributed EdgeLake deployments.

### Configuration Strategy

#### Option 1: OVERLAY_IP (Recommended)

**Concept**: Use Tailscale IPs for all node-to-node communication

**Configuration**:

**Master** (`master-configs/advance_configs.env`):
```bash
OVERLAY_IP=100.64.1.10    # Master's Tailscale IP
```

**Master** (`master-configs/base_configs.env`):
```bash
LEDGER_CONN=127.0.0.1:32048    # Points to itself (localhost)
```

**Operator** (`operator-configs/advance_configs.env`):
```bash
OVERLAY_IP=100.64.1.20    # Operator's Tailscale IP
```

**Operator** (`operator-configs/base_configs.env`):
```bash
LEDGER_CONN=100.64.1.10:32048    # Master's Tailscale IP
```

**Query** (`query-configs/advance_configs.env`):
```bash
OVERLAY_IP=100.64.1.30    # Query's Tailscale IP
```

**Query** (`query-configs/base_configs.env`):
```bash
LEDGER_CONN=100.64.1.10:32048    # Master's Tailscale IP
```

**How it works**:
- `OVERLAY_IP` sets the `local_ip` field in policies
- All EdgeLake commands use `!overlay_ip` for binding
- TCP/REST servers bind to Tailscale interface
- Nodes discover each other via `local_ip` field in policies

**Resulting Node Policy** (Operator):
```json
{
  "operator": {
    "name": "edgelake-operator",
    "ip": "23.45.67.89",          // External IP (auto-detected, may be NAT)
    "local_ip": "100.64.1.20",    // Tailscale IP (used for communication)
    "port": 32148,
    "rest_port": 32149,
    ...
  }
}
```

#### Option 2: TCP_BIND with OVERLAY_IP

**Concept**: Explicitly bind TCP server to Tailscale IP

**Configuration** (`advance_configs.env`):
```bash
OVERLAY_IP=100.64.1.20
```

**Configuration** (`base_configs.env`):
```bash
TCP_BIND=true              # Bind TCP server to specific IP
```

**Result**: Policy uses `ip` field (not `local_ip`) for Tailscale address:
```json
{
  "operator": {
    "ip": "100.64.1.20",     // Tailscale IP in main 'ip' field
    "port": 32148,
    ...
  }
}
```

### Testing Tailscale Connectivity

```bash
# 1. Get Tailscale IPs
tailscale status

# Example output:
# 100.64.1.10  master-host      user@   linux   -
# 100.64.1.20  operator-host    user@   linux   -
# 100.64.1.30  query-host       user@   linux   -

# 2. Test connectivity from Operator to Master
ping 100.64.1.10

# 3. Test EdgeLake TCP port
nc -zv 100.64.1.10 32048

# 4. Test EdgeLake REST port
curl http://100.64.1.10:32049

# 5. Check EdgeLake connection from inside container
docker exec edgelake-operator bash -c "curl http://100.64.1.10:32049"
```

### Network Mode Consideration

**Docker Compose uses `network_mode: host`**:
- Container shares host's network namespace
- Container can directly access Tailscale interface
- No port mapping needed (ports exposed directly on host)

**Implication**: Tailscale must be running on the **Docker host**, not inside the container

---

## Configuration Reference

### Environment Variables by Category

#### Core Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `NODE_TYPE` | ✅ | - | `master`, `operator`, `query`, `generic` |
| `NODE_NAME` | ✅ | `edgelake-{type}` | Unique node identifier |
| `COMPANY_NAME` | ✅ | `"New Company"` | Company/organization name |
| `LEDGER_CONN` | ✅ (non-master) | `127.0.0.1:32048` | Master node connection (`IP:PORT`) |

#### Networking

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OVERLAY_IP` | ⚠️ Tailscale | - | Tailscale/VPN IP address |
| `ANYLOG_SERVER_PORT` | ✅ | `32{0,1,2,3}48` | TCP protocol port |
| `ANYLOG_REST_PORT` | ✅ | `32{0,1,2,3}49` | REST API port |
| `ANYLOG_BROKER_PORT` | ❌ | - | MQTT broker port (optional) |
| `TCP_BIND` | ❌ | `false` | Bind TCP to specific IP |
| `REST_BIND` | ❌ | `false` | Bind REST to specific IP |
| `TCP_THREADS` | ❌ | `6` | TCP worker threads |
| `REST_THREADS` | ❌ | `6` | REST worker threads |
| `REST_TIMEOUT` | ❌ | `30` | REST request timeout (seconds) |

**Port Defaults by Node Type**:
- **Master**: `32048` (TCP), `32049` (REST)
- **Operator**: `32148` (TCP), `32149` (REST)
- **Query**: `32348` (TCP), `32349` (REST)
- **Generic**: `32548` (TCP), `32549` (REST)

#### Database

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DB_TYPE` | ❌ | `sqlite` | `sqlite`, `psql`, `mongo` |
| `DB_USER` | ⚠️ psql | - | Database username (PostgreSQL) |
| `DB_PASSWD` | ⚠️ psql | - | Database password (PostgreSQL) |
| `DB_IP` | ⚠️ psql | `127.0.0.1` | Database host IP |
| `DB_PORT` | ⚠️ psql | `5432` | Database port |
| `SYSTEM_QUERY` | ❌ | varies | Enable system_query database |
| `MEMORY` | ❌ | varies | Use in-memory SQLite |

#### Blockchain/Metadata

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `BLOCKCHAIN_SOURCE` | ❌ | `master` | Metadata source (`master`, `optimism`, etc.) |
| `BLOCKCHAIN_SYNC` | ❌ | `30 second` | Metadata sync interval |
| `BLOCKCHAIN_DESTINATION` | ❌ | `file` | Storage location for metadata |

#### Operator-Specific

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `CLUSTER_NAME` | ✅ (operator) | - | Cluster identifier |
| `DEFAULT_DBMS` | ✅ (operator) | - | Default database name |
| `ENABLE_PARTITIONS` | ❌ | `true` | Enable table partitioning |
| `PARTITION_INTERVAL` | ❌ | `14 days` | Partition time window |
| `ENABLE_HA` | ❌ | `false` | Enable high availability |
| `OPERATOR_THREADS` | ❌ | `3` | Operator worker threads |

### Example Configurations

#### Master Node (Tailscale)

**`master-configs/base_configs.env`**:
```bash
NODE_TYPE=master
NODE_NAME=edgelake-master
COMPANY_NAME="Acme Corp"
ANYLOG_SERVER_PORT=32048
ANYLOG_REST_PORT=32049
LEDGER_CONN=127.0.0.1:32048       # Self-reference
DB_TYPE=sqlite
BLOCKCHAIN_SOURCE=master
```

**`master-configs/advance_configs.env`**:
```bash
OVERLAY_IP=100.64.1.10             # Tailscale IP
TCP_THREADS=6
REST_THREADS=6
REST_TIMEOUT=30
```

#### Operator Node (Tailscale)

**`operator-configs/base_configs.env`**:
```bash
NODE_TYPE=operator
NODE_NAME=edgelake-operator-1
COMPANY_NAME="Acme Corp"
ANYLOG_SERVER_PORT=32148
ANYLOG_REST_PORT=32149
LEDGER_CONN=100.64.1.10:32048     # Master's Tailscale IP
DB_TYPE=sqlite
BLOCKCHAIN_SOURCE=master
CLUSTER_NAME=acme-cluster-1
DEFAULT_DBMS=acme_data
ENABLE_PARTITIONS=true
```

**`operator-configs/advance_configs.env`**:
```bash
OVERLAY_IP=100.64.1.20             # Operator's Tailscale IP
TCP_THREADS=6
REST_THREADS=6
OPERATOR_THREADS=3
PARTITION_INTERVAL=14 days
PARTITION_KEEP=3
```

#### Query Node (Tailscale)

**`query-configs/base_configs.env`**:
```bash
NODE_TYPE=query
NODE_NAME=edgelake-query
COMPANY_NAME="Acme Corp"
ANYLOG_SERVER_PORT=32348
ANYLOG_REST_PORT=32349
LEDGER_CONN=100.64.1.10:32048     # Master's Tailscale IP
DB_TYPE=sqlite
BLOCKCHAIN_SOURCE=master
SYSTEM_QUERY=true
MEMORY=true
```

**`query-configs/advance_configs.env`**:
```bash
OVERLAY_IP=100.64.1.30             # Query's Tailscale IP
TCP_THREADS=6
REST_THREADS=6
QUERY_POOL=6
MONITOR_NODES=true
```

---

## Troubleshooting

### Common Issues

#### 1. Nodes Can't Find Master

**Symptom**: `Failed to connect to blockchain` or `Connection refused`

**Check**:
```bash
# Verify LEDGER_CONN is correct
docker exec edgelake-operator printenv LEDGER_CONN

# Test connectivity to Master
docker exec edgelake-operator curl http://100.64.1.10:32049

# Check Master is running
docker logs edgelake-master | grep "run rest server"
```

**Fix**: Update `LEDGER_CONN` in `base_configs.env` to Master's Tailscale IP

#### 2. Policies Not Syncing

**Symptom**: `blockchain get operator` returns empty

**Check**:
```bash
# Check blockchain sync status
docker exec edgelake-query python3 /app/EdgeLake/edge_lake/edgelake.py "blockchain get (blockchain) bring.recent"

# Check connection to Master
docker exec edgelake-query python3 /app/EdgeLake/edge_lake/edgelake.py "test network !ledger_conn"
```

**Fix**: Verify `BLOCKCHAIN_SYNC` interval and Master connectivity

#### 3. Wrong IP in Policies

**Symptom**: Nodes registered with `127.0.0.1` or wrong IP

**Check**:
```bash
# View published policy
docker exec edgelake-operator python3 /app/EdgeLake/edge_lake/edgelake.py "blockchain get operator bring.recent"
```

**Fix**: Set `OVERLAY_IP` in `advance_configs.env`

#### 4. Tailscale Not Working

**Symptom**: Can't ping Tailscale IPs from container

**Check**:
```bash
# Tailscale must run on Docker host (not in container)
tailscale status

# Test from host
ping 100.64.1.10

# Test from container (should work due to network_mode: host)
docker exec edgelake-operator ping 100.64.1.10
```

**Fix**: Ensure Tailscale is running on host, not inside container

---

## Summary

### Key Takeaways

1. **Docker Compose** provides the **infrastructure layer**:
   - Container orchestration
   - Environment variable injection
   - Volume management

2. **Deployment Scripts** provide the **configuration logic**:
   - Read environment variables
   - Build policy JSON structures
   - Publish policies to Master
   - Start EdgeLake services

3. **Environment variables flow**:
   ```
   .env files → Docker Compose → Container ($VARS) → set_params.al (conversion) → !local_vars → Policies
   ```

4. **Policies enable**:
   - Configuration persistence
   - Node discovery
   - Network coordination

5. **Tailscale integration**:
   - Set `OVERLAY_IP` to Tailscale IP
   - Set `LEDGER_CONN` to Master's Tailscale IP
   - Nodes discover each other via policies
   - All communication happens over Tailscale mesh

### Next Steps

- Set your Tailscale IPs in `OVERLAY_IP` variables
- Update `LEDGER_CONN` to point to Master's Tailscale IP
- Deploy Master node first: `make up master`
- Deploy Operator/Query nodes: `make up operator`, `make up query`
- Verify policies: `blockchain get operator`
- Test distributed queries

---

**Document Version**: 1.0
**Last Updated**: 2025-01-05
**Author**: Claude Code (with user collaboration)
