# Docker Volume Naming in EdgeLake Deployments

## Overview

Docker Compose automatically adds a **project prefix** to volume names, which can cause confusion when migrating from demo/OVA deployments to production setups. This guide explains how volume naming works and how to preserve existing data volumes.

---

## Docker Volume Naming Convention

### Template Definition

In `docker-compose-template-base.yaml`:
```yaml
services:
  ${NODE_NAME}:
    volumes:
      - ${NODE_NAME}-anylog:/app/EdgeLake/anylog
      - ${NODE_NAME}-blockchain:/app/EdgeLake/blockchain
      - ${NODE_NAME}-data:/app/EdgeLake/data
      - ${NODE_NAME}-local-scripts:/app/deployment-scripts

volumes:
  ${NODE_NAME}-anylog:
  ${NODE_NAME}-blockchain:
  ${NODE_NAME}-data:
  ${NODE_NAME}-local-scripts:
```

**Template Variable**: `${NODE_NAME}` = `edgelake-master` (example)

**Expected Volume Names** (from template):
- `edgelake-master-anylog`
- `edgelake-master-blockchain`
- `edgelake-master-data`
- `edgelake-master-local-scripts`

### Actual Volume Names (with Docker Compose Prefix)

When Docker Compose runs, it adds a **project prefix** based on:
1. The directory containing the compose file
2. Or the `-p/--project-name` flag (if specified)

**Formula**: `<project-prefix>_<volume-name>`

**Example** (running from `docker-makefiles/` directory):
- `docker-makefiles_edgelake-master-anylog`
- `docker-makefiles_edgelake-master-blockchain`
- `docker-makefiles_edgelake-master-data`
- `docker-makefiles_edgelake-master-local-scripts`

---

## Understanding Your Current Volumes

Based on your volume list:

### 1. `docker-compose-files_remote-cli-current`

**Breakdown**:
- **Prefix**: `docker-compose-files` (compose file was in `docker-compose-files/` directory)
- **Volume Name**: `remote-cli-current`
- **Type**: Remote CLI interface volume
- **Source**: Likely from an earlier deployment or OVA demo

### 2. `docker-makefiles_edgelake-demo-master-anylog`

**Breakdown**:
- **Prefix**: `docker-makefiles` (compose file in `docker-makefiles/` directory)
- **Node Name**: `edgelake-demo-master`
- **Volume Type**: `anylog` (configuration and logs)
- **Source**: Demo Master node from OVA

**Other Related Volumes** (expected):
- `docker-makefiles_edgelake-demo-master-blockchain` (metadata policies)
- `docker-makefiles_edgelake-demo-master-data` (database files - likely empty for Master)
- `docker-makefiles_edgelake-demo-master-local-scripts` (deployment scripts)

---

## Volume Contents by Type

### 1. `${NODE_NAME}-anylog` Volume

**Mount Point**: `/app/EdgeLake/anylog`

**Contents**:
```
anylog/
├── EdgeLake.log              # Main log file
├── error.log                 # Error log
├── query.log                 # Query execution log
├── rest_log.txt              # REST API log
├── dbms/                     # Database metadata
├── watch/                    # File monitoring
└── pem/                      # SSL certificates (if used)
```

**Importance**: **LOW** for data recovery (logs and config, can be regenerated)

### 2. `${NODE_NAME}-blockchain` Volume

**Mount Point**: `/app/EdgeLake/blockchain`

**Contents**:
```
blockchain/
├── metadata/                 # Policy JSON files
│   ├── config_*.json         # Config policies
│   ├── operator_*.json       # Operator node policies
│   ├── cluster_*.json        # Cluster policies
│   ├── table_*.json          # Table schema policies
│   └── mapping_*.json        # Data mapping policies
└── sync_log.txt              # Sync status
```

**Importance**: **HIGH** - Contains all network metadata and policies

**Critical Files**:
- **Cluster policies**: Define operator clusters and data distribution
- **Table policies**: Define table schemas (required for queries)
- **Operator policies**: Node registry (IP addresses, ports)
- **Mapping policies**: Data ingestion mappings

**Recovery**: Can be regenerated from Master node's blockchain volume

### 3. `${NODE_NAME}-data` Volume

**Mount Point**: `/app/EdgeLake/data`

**Contents**:
```
data/
├── dbms/                     # Database files (SQLite, etc.)
│   ├── <dbms_name>/          # Per-database directory
│   │   ├── <table_name>/     # Per-table directory
│   │   │   ├── *.db          # SQLite database files
│   │   │   ├── *.par         # Partition metadata
│   │   │   └── json/         # Archived JSON data
│   │   └── system_query.db   # System metadata DB
├── archive/                  # Archived data files
│   ├── json/                 # JSON archives
│   └── sql/                  # SQL archives
├── blobs/                    # Binary large objects
├── bkup/                     # Backups
└── error/                    # Failed ingestion data
```

**Importance**: **CRITICAL** - Contains all your operational data

**For Operator Nodes**:
- This is where your time-series data lives
- SQLite databases or PostgreSQL data
- Table partitions
- Historical archives

**For Master/Query Nodes**:
- Usually minimal data (mostly system_query DB)
- Not critical for data recovery

### 4. `${NODE_NAME}-local-scripts` Volume

**Mount Point**: `/app/deployment-scripts`

**Contents**:
```
deployment-scripts/
├── node-deployment/          # Standard deployment scripts
│   ├── main.al
│   ├── set_params.al
│   └── policies/
└── custom-scripts/           # User-added scripts (if any)
```

**Importance**: **LOW** - Standard scripts, can be regenerated

---

## Preserving Existing Data Volumes

### Scenario: Migrating from Demo OVA to Production

You have:
- **Old Operator**: `docker-makefiles_edgelake-demo-operator-data` (contains operational tables)
- **Old Master**: `docker-makefiles_edgelake-demo-master-blockchain` (contains policies)

You want:
- **New Operator**: `docker-makefiles_edgelake-operator-data` (preserve old data)
- **New Master**: `docker-makefiles_edgelake-master-blockchain` (preserve policies)

### Strategy 1: Rename NODE_NAME to Match Existing Volumes

**Idea**: Keep the old volume names by using the same `NODE_NAME`

**Configuration** (`operator-configs/base_configs.env`):
```bash
NODE_NAME=edgelake-demo-operator    # Matches existing volume prefix
```

**Result**:
- Docker Compose will use existing `docker-makefiles_edgelake-demo-operator-data` volume
- No data migration needed
- Container name will be `edgelake-demo-operator`

**Pros**:
- No volume copying needed
- Immediate access to existing data
- Zero downtime

**Cons**:
- Less clean naming (keeps "demo" in name)
- Doesn't allow easy migration to new naming scheme

### Strategy 2: Copy Data Between Volumes

**Idea**: Create new volumes with clean names, copy data from old volumes

**Steps**:

#### Step 1: Identify Volumes to Preserve

```bash
# List all volumes
docker volume ls

# Identify critical volumes:
# - docker-makefiles_edgelake-demo-operator-data (CRITICAL - operational data)
# - docker-makefiles_edgelake-demo-master-blockchain (HIGH - policies)
```

#### Step 2: Create New Deployment (generates new volumes)

```bash
cd docker-compose
make up operator EDGELAKE_TYPE=operator
```

This creates:
- `docker-makefiles_edgelake-operator-anylog`
- `docker-makefiles_edgelake-operator-blockchain`
- `docker-makefiles_edgelake-operator-data` (empty)
- `docker-makefiles_edgelake-operator-local-scripts`

#### Step 3: Stop New Container

```bash
docker stop edgelake-operator
```

#### Step 4: Copy Data from Old Volume to New Volume

**Using a temporary container**:

```bash
# Copy operator data volume
docker run --rm -it \
  -v docker-makefiles_edgelake-demo-operator-data:/source:ro \
  -v docker-makefiles_edgelake-operator-data:/dest \
  alpine sh -c "cp -av /source/* /dest/"

# Copy operator blockchain volume (if exists)
docker run --rm -it \
  -v docker-makefiles_edgelake-demo-operator-blockchain:/source:ro \
  -v docker-makefiles_edgelake-operator-blockchain:/dest \
  alpine sh -c "cp -av /source/* /dest/"

# Copy master blockchain volume (policies)
docker run --rm -it \
  -v docker-makefiles_edgelake-demo-master-blockchain:/source:ro \
  -v docker-makefiles_edgelake-master-blockchain:/dest \
  alpine sh -c "cp -av /source/* /dest/"
```

**Verification**:

```bash
# Check data volume size (should match old volume)
docker run --rm -v docker-makefiles_edgelake-operator-data:/data alpine du -sh /data

# Inspect contents
docker run --rm -v docker-makefiles_edgelake-operator-data:/data alpine ls -lah /data/dbms
```

#### Step 5: Restart Container with Preserved Data

```bash
docker start edgelake-operator

# Or restart via make
make up operator
```

#### Step 6: Verify Data Access

```bash
# Attach to container
docker exec -it edgelake-operator python3 /app/EdgeLake/edge_lake/edgelake.py

# Check databases
AL > get databases

# Check tables
AL > get tables where dbms = <your_dbms_name>

# Test query
AL > sql <your_dbms_name> SELECT COUNT(*) FROM <your_table>
```

### Strategy 3: Use Docker Compose Project Name Override

**Idea**: Force Docker Compose to use a specific project name

**Method 1: Environment Variable**:

```bash
# Set project name before running make
export COMPOSE_PROJECT_NAME=edgelake

# Deploy
make up operator

# Result: Volumes will be named:
# - edgelake_edgelake-operator-anylog
# - edgelake_edgelake-operator-blockchain
# - edgelake_edgelake-operator-data
# - edgelake_edgelake-operator-local-scripts
```

**Method 2: Modify Makefile**:

Edit `docker-compose/Makefile` line 186:

```makefile
# Before:
@$(DOCKER_COMPOSE_CMD) -f docker-makefiles/docker-compose-files/${DOCKER_FILE_NAME} up --build -d

# After (add -p flag):
@$(DOCKER_COMPOSE_CMD) -p edgelake -f docker-makefiles/docker-compose-files/${DOCKER_FILE_NAME} up --build -d
```

**Result**: All volumes use `edgelake_` prefix instead of `docker-makefiles_`

---

## Recommended Migration Plan

### For Operator Nodes (Data Preservation Critical)

**Recommended**: **Strategy 2** (Copy Data Between Volumes)

**Why**:
- Clean naming scheme (`edgelake-operator` instead of `edgelake-demo-operator`)
- Preserves original data as backup
- Allows validation before deleting old volumes
- Minimal risk

**Steps**:
1. Deploy new operator node (creates empty volumes)
2. Stop new container
3. Copy data from old `data` volume to new `data` volume
4. Copy policies from old `blockchain` volume to new `blockchain` volume (optional, can sync from Master)
5. Restart container
6. Verify tables and data are accessible
7. (Later) Delete old demo volumes once confident

### For Master Node (Policy Preservation)

**Recommended**: **Strategy 2** (Copy Blockchain Volume) **OR** **Strategy 1** (Rename to Match)

**Why**:
- Master node doesn't have operational data (just policies)
- Policies can be republished if lost (but tedious)
- Blockchain volume is small (usually < 100MB)

**Steps**:
1. Deploy new master node
2. Stop new container
3. Copy `blockchain` volume from demo master to new master
4. Restart container
5. Verify policies: `blockchain get operator`

### For Query Nodes (No Data Preservation Needed)

**Recommended**: **Fresh Deployment**

**Why**:
- Query nodes don't store operational data
- Only have `system_query` database (recreatable)
- No migration needed

---

## Volume Inspection Commands

### List All Volumes

```bash
docker volume ls
```

### Inspect Volume Details

```bash
# Get mount point and size
docker volume inspect docker-makefiles_edgelake-demo-operator-data

# Output shows:
# - Mountpoint: /var/lib/docker/volumes/docker-makefiles_edgelake-demo-operator-data/_data
# - Driver: local
```

### Check Volume Size

```bash
docker run --rm -v docker-makefiles_edgelake-demo-operator-data:/data alpine du -sh /data
```

### Browse Volume Contents (Interactive)

```bash
docker run --rm -it -v docker-makefiles_edgelake-demo-operator-data:/data alpine sh

# Inside container:
ls -lah /data
ls -lah /data/dbms
ls -lah /data/archive
exit
```

### List Tables in Data Volume

```bash
docker run --rm -v docker-makefiles_edgelake-demo-operator-data:/data alpine sh -c "find /data/dbms -type d -maxdepth 3"

# Example output:
# /data/dbms
# /data/dbms/my_company
# /data/dbms/my_company/sensor_data
# /data/dbms/my_company/device_status
```

### Check SQLite Database Files

```bash
docker run --rm -v docker-makefiles_edgelake-demo-operator-data:/data alpine sh -c "find /data -name '*.db' -exec ls -lh {} \;"
```

---

## Volume Cleanup (After Migration)

### Remove Old Demo Volumes (After Verifying New Deployment)

```bash
# ⚠️ WARNING: This deletes data permanently!

# Remove old operator data volume
docker volume rm docker-makefiles_edgelake-demo-operator-data

# Remove old operator blockchain volume
docker volume rm docker-makefiles_edgelake-demo-operator-blockchain

# Remove old master volumes
docker volume rm docker-makefiles_edgelake-demo-master-anylog
docker volume rm docker-makefiles_edgelake-demo-master-blockchain
docker volume rm docker-makefiles_edgelake-demo-master-data
docker volume rm docker-makefiles_edgelake-demo-master-local-scripts
```

### Remove Unused Volumes (Safe)

```bash
# Remove volumes not attached to any container
docker volume prune

# Confirm before deleting
```

---

## Volume Backup Best Practices

### Before Migration

```bash
# Backup critical operator data volume
docker run --rm -v docker-makefiles_edgelake-demo-operator-data:/data \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/operator-data-backup-$(date +%Y%m%d).tar.gz -C /data .

# Backup master blockchain volume
docker run --rm -v docker-makefiles_edgelake-demo-master-blockchain:/blockchain \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/master-blockchain-backup-$(date +%Y%m%d).tar.gz -C /blockchain .
```

### Restore from Backup (if needed)

```bash
# Restore operator data
docker run --rm -v docker-makefiles_edgelake-operator-data:/data \
  -v $(pwd)/backups:/backup \
  alpine sh -c "cd /data && tar xzf /backup/operator-data-backup-20250105.tar.gz"

# Restore master blockchain
docker run --rm -v docker-makefiles_edgelake-master-blockchain:/blockchain \
  -v $(pwd)/backups:/backup \
  alpine sh -c "cd /blockchain && tar xzf /backup/master-blockchain-backup-20250105.tar.gz"
```

---

## Troubleshooting

### Issue: "Database not found" after migration

**Cause**: Data volume wasn't copied correctly

**Fix**:
```bash
# Check if data volume has content
docker run --rm -v docker-makefiles_edgelake-operator-data:/data alpine ls -lah /data/dbms

# If empty, re-copy from old volume
docker run --rm -v docker-makefiles_edgelake-demo-operator-data:/source:ro \
  -v docker-makefiles_edgelake-operator-data:/dest \
  alpine sh -c "cp -av /source/* /dest/"
```

### Issue: "Table schema not found" errors

**Cause**: Blockchain policies missing

**Fix Option 1** - Copy from old master:
```bash
docker run --rm -v docker-makefiles_edgelake-demo-master-blockchain:/source:ro \
  -v docker-makefiles_edgelake-operator-blockchain:/dest \
  alpine sh -c "cp -av /source/metadata/* /dest/metadata/"
```

**Fix Option 2** - Sync from current Master:
```bash
# Inside operator container
AL > blockchain sync where source=master and connection=<master_ip>:32048
```

### Issue: Volume name mismatch

**Cause**: Docker Compose project name changed

**Fix**: Use explicit project name:
```bash
# Find current project name from volume prefix
docker volume ls | grep edgelake

# Use matching project name
export COMPOSE_PROJECT_NAME=docker-makefiles
make up operator
```

---

## Summary

### Volume Naming Formula

```
<compose-project-prefix>_<node-name>-<volume-type>
       ↓                      ↓           ↓
docker-makefiles_    edgelake-operator-  data

Where:
- compose-project-prefix: Directory containing compose file (or -p flag value)
- node-name: $NODE_NAME from base_configs.env
- volume-type: anylog, blockchain, data, or local-scripts
```

### Critical Volumes to Preserve

| Volume Type | Importance | Contains | Recovery Difficulty |
|-------------|------------|----------|---------------------|
| `*-data` (Operator) | **CRITICAL** | Operational tables, time-series data | Hard - data loss |
| `*-blockchain` (Master) | **HIGH** | Network policies, schemas | Medium - can republish |
| `*-blockchain` (Operator) | **MEDIUM** | Local policy cache | Easy - sync from Master |
| `*-anylog` | **LOW** | Logs, temp files | Easy - regenerated |
| `*-local-scripts` | **LOW** | Standard scripts | Easy - regenerated |

### Recommended Migration Workflow

1. **Backup critical volumes** (operator data, master blockchain)
2. **Deploy new nodes** with clean naming
3. **Stop new containers**
4. **Copy data volumes** from old to new
5. **Restart containers**
6. **Verify** tables and policies are accessible
7. **Delete old volumes** (after 1-2 weeks of successful operation)

---

**Document Version**: 1.0
**Last Updated**: 2025-01-05
**Author**: Claude Code (with user collaboration)
