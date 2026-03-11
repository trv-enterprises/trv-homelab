# EdgeLake Data Loading Guide

## Overview

EdgeLake supports multiple methods for loading data into Operator nodes. This guide covers all the ways to get your data into EdgeLake tables.

---

## Table of Contents

1. [Quick Start - REST PUT Method](#quick-start---rest-put-method)
2. [Watch Directory Method](#watch-directory-method)
3. [MQTT Streaming](#mqtt-streaming)
4. [gRPC Integration](#grpc-integration)
5. [Understanding the Data Flow](#understanding-the-data-flow)
6. [Directory Structure](#directory-structure)
7. [Data Format Requirements](#data-format-requirements)
8. [Troubleshooting](#troubleshooting)

---

## Quick Start - REST PUT Method

### What is it?

The **fastest way** to load data - send JSON directly to an Operator node's REST API.

### Requirements

- Operator node running with REST server active (default port: 32149)
- JSON data formatted correctly
- Table name and database name

### Basic Example

```bash
curl -X PUT http://localhost:32149 \
  -H "type: json" \
  -H "dbms: my_database" \
  -H "table: sensor_data" \
  -H "Content-Type: application/json" \
  -d '[
    {
      "timestamp": "2025-01-05T10:30:00Z",
      "device_name": "sensor_001",
      "temperature": 72.5,
      "humidity": 45.2
    },
    {
      "timestamp": "2025-01-05T10:31:00Z",
      "device_name": "sensor_002",
      "temperature": 68.3,
      "humidity": 50.1
    }
  ]'
```

### Response

**Success**:
```json
{
  "AnyLog.status": "Success",
  "AnyLog.hash": "abc123def456..."
}
```

**Error**:
```json
{
  "AnyLog.error on REST server": "Missing 'dbms' name in REST PUT command"
}
```

### Required Headers

| Header | Required | Example | Description |
|--------|----------|---------|-------------|
| `type` | ✅ | `json` | Data format (json, sql, etc.) |
| `dbms` | ✅ | `my_database` | Target database name |
| `table` | ✅ | `sensor_data` | Target table name |
| `Content-Type` | ✅ | `application/json` | Request content type |
| `source` | ❌ | `mqtt` | Data source identifier (optional) |
| `instructions` | ❌ | `mapping_policy_id` | Mapping policy ID (optional) |

### Multiple Records Example

```bash
# Load 1000 records at once
curl -X PUT http://100.64.1.20:32149 \
  -H "type: json" \
  -H "dbms: iot_data" \
  -H "table: device_readings" \
  -H "Content-Type: application/json" \
  -d @data.json
```

Where `data.json` contains:
```json
[
  {"timestamp": "2025-01-05T10:00:00Z", "device": "dev1", "value": 42.5},
  {"timestamp": "2025-01-05T10:01:00Z", "device": "dev2", "value": 38.1},
  ...
]
```

---

## Watch Directory Method

### What is it?

EdgeLake can **automatically monitor a directory** and ingest any JSON files placed there. This is ideal for:
- Batch data loading
- Scheduled imports (cron jobs)
- Third-party data dumps
- Migration from other systems

### How It Works

```
┌────────────────────────────────────────────────────────┐
│ 1. User/Script places JSON file in watch directory    │
│    /app/EdgeLake/anylog/watch/my_data.json             │
└────────────────────────────────────────────────────────┘
                        ↓
┌────────────────────────────────────────────────────────┐
│ 2. EdgeLake Streamer detects new file                 │
│    (runs every few seconds)                            │
└────────────────────────────────────────────────────────┘
                        ↓
┌────────────────────────────────────────────────────────┐
│ 3. File moved to prep directory for processing        │
│    /app/EdgeLake/anylog/prep/my_data.json              │
└────────────────────────────────────────────────────────┘
                        ↓
┌────────────────────────────────────────────────────────┐
│ 4. Data parsed and inserted into database             │
│    - Schema auto-created if needed                     │
│    - Data validated                                    │
└────────────────────────────────────────────────────────┘
                        ↓
┌────────────────────────────────────────────────────────┐
│ 5. File archived or moved to error directory          │
│    Success: /app/EdgeLake/data/archive/json/           │
│    Error:   /app/EdgeLake/data/error/                  │
└────────────────────────────────────────────────────────┘
```

### Setup Requirements

#### Step 1: Ensure Streamer is Running

The streamer process watches the directories for new files.

**Check if running**:
```bash
docker exec edgelake-operator python3 /app/EdgeLake/edge_lake/edgelake.py
AL > get processes
```

**Look for**:
```
Streamer       | Running
```

**If not running**:
```bash
AL > run streamer
```

This command is usually in your deployment config policy already (line from `config_policy.al`):
```al
"run streamer"
```

#### Step 2: Verify Watch Directory Paths

**Check current paths**:
```bash
AL > get dictionary _dir
```

**Expected output**:
```
!anylog_path  = /app/EdgeLake
!prep_dir     = /app/EdgeLake/anylog/prep
!watch_dir    = /app/EdgeLake/anylog/watch
!bwatch_dir   = /app/EdgeLake/anylog/bwatch
!err_dir      = /app/EdgeLake/data/error
!archive_dir  = /app/EdgeLake/data/archive
```

#### Step 3: Place JSON Files in Watch Directory

**From inside container**:
```bash
docker exec -it edgelake-operator sh

# Create sample JSON file
cat > /app/EdgeLake/anylog/watch/sensor_data.json <<EOF
[
  {
    "timestamp": "2025-01-05T10:00:00Z",
    "device_name": "sensor_001",
    "temperature": 72.5,
    "humidity": 45.2,
    "location": "Building A"
  },
  {
    "timestamp": "2025-01-05T10:01:00Z",
    "device_name": "sensor_002",
    "temperature": 68.3,
    "humidity": 50.1,
    "location": "Building B"
  }
]
EOF

exit
```

**From host machine** (using Docker volume):
```bash
# Find the watch directory volume
docker volume inspect docker-makefiles_edgelake-operator-anylog | grep Mountpoint

# Example output:
# "Mountpoint": "/var/lib/docker/volumes/docker-makefiles_edgelake-operator-anylog/_data"

# Copy file to watch directory
sudo cp my_data.json /var/lib/docker/volumes/docker-makefiles_edgelake-operator-anylog/_data/watch/
```

**Using Docker cp** (easier):
```bash
# Copy from host to container's watch directory
docker cp my_data.json edgelake-operator:/app/EdgeLake/anylog/watch/
```

#### Step 4: File Naming Convention

Files in the watch directory should follow this naming pattern for **automatic table routing**:

```
<dbms_name>.<table_name>.<any_unique_id>.json
```

**Examples**:
```bash
# Database: iot_data, Table: sensor_readings
iot_data.sensor_readings.001.json
iot_data.sensor_readings.2025-01-05.json
iot_data.sensor_readings.sensor001.json

# Database: manufacturing, Table: machine_status
manufacturing.machine_status.plant1.json
```

**Why this format?**
- EdgeLake automatically extracts `dbms_name` and `table_name` from the filename
- No need to specify them manually
- Files are automatically routed to the correct table

**Alternative**: If your files don't follow this convention, you need a **mapping policy** (covered later).

### Monitoring Watch Directory Activity

**Check streamer status**:
```bash
AL > get streaming
```

**View recently processed files**:
```bash
AL > get files in archive
```

**Check for errors**:
```bash
AL > get files in error
```

**View logs**:
```bash
docker exec edgelake-operator tail -f /app/EdgeLake/anylog/EdgeLake.log
```

---

## MQTT Streaming

### What is it?

Receive continuous data streams from **MQTT brokers** (IoT devices, sensors, applications).

### Setup Example

**Configuration** (in `operator-configs/base_configs.env`):
```bash
# Enable MQTT client
ENABLE_MQTT=true

# MQTT broker connection
MQTT_BROKER=mqtt.example.com
MQTT_PORT=1883
MQTT_USER=myuser
MQTT_PASSWD=mypassword

# Data routing
MSG_TOPIC=sensor/+/data
MSG_DBMS=iot_data
MSG_TABLE=sensor_readings
MSG_TIMESTAMP_COLUMN=timestamp
MSG_VALUE_COLUMN=value
```

**Manual setup** (via EdgeLake CLI):
```bash
AL > run msg client where broker=mqtt.example.com and port=1883 and user=myuser and password=mypassword

AL > subscribe where topic=sensor/+/data and dbms=iot_data and table=sensor_readings
```

### MQTT Message Format

EdgeLake expects JSON payloads:
```json
{
  "timestamp": "2025-01-05T10:30:00Z",
  "device_id": "sensor_001",
  "temperature": 72.5,
  "humidity": 45.2
}
```

**Published to topic**: `sensor/building-a/data`

**Automatically inserted into**: `iot_data.sensor_readings` table

---

## gRPC Integration

### What is it?

High-performance binary protocol for streaming data from applications.

### Example

**Start gRPC client**:
```bash
AL > run grpc client where ip=grpc.server.com and port=50051
```

**Configure data mapping**:
```bash
AL > set grpc mapping where dbms=iot_data and table=device_data
```

---

## Understanding the Data Flow

### Directory Flow (Watch Method)

```
User/Application
       ↓ (places file)
!watch_dir (/app/EdgeLake/anylog/watch/)
       ↓ (streamer detects)
!prep_dir (/app/EdgeLake/anylog/prep/)
       ↓ (processing)
Database Tables
       ↓ (after processing)
!archive_dir (/app/EdgeLake/data/archive/json/)  [SUCCESS]
  OR
!err_dir (/app/EdgeLake/data/error/)              [FAILURE]
```

### REST PUT Flow

```
Client Application
       ↓ (HTTP PUT)
REST Server (port 32149)
       ↓ (validates headers)
Streaming Data Handler
       ↓ (writes to prep_dir)
!prep_dir (/app/EdgeLake/anylog/prep/)
       ↓ (processing)
Database Tables
       ↓ (after processing)
Archive or Error directory
```

### Streamer Process

The **streamer** is a background process that:
1. Monitors `!watch_dir` for new files
2. Moves files to `!prep_dir`
3. Parses JSON and validates structure
4. Inserts data into database tables
5. Archives successful files, logs errors

**Start streamer**:
```bash
AL > run streamer
```

**Check status**:
```bash
AL > get processes
```

---

## Directory Structure

### Standard EdgeLake Directories

```
/app/EdgeLake/
├── anylog/                      # Configuration and working directories
│   ├── EdgeLake.log             # Main log file
│   ├── error.log                # Error log
│   ├── prep/                    # !prep_dir - Files being processed
│   ├── watch/                   # !watch_dir - Files waiting to be processed
│   ├── bwatch/                  # !bwatch_dir - Binary watch directory
│   └── dbms/                    # Database metadata
├── data/
│   ├── dbms/                    # SQLite database files
│   │   ├── <database_name>/
│   │   │   ├── <table_name>/
│   │   │   │   └── *.db         # Table data files
│   ├── archive/
│   │   └── json/                # !archive_dir - Successfully processed files
│   └── error/                   # !err_dir - Failed files (for review)
└── blockchain/
    └── metadata/                # Network policies
```

### Docker Volume Mapping

When running in Docker:
```yaml
volumes:
  - edgelake-operator-anylog:/app/EdgeLake/anylog     # Contains watch/prep directories
  - edgelake-operator-data:/app/EdgeLake/data         # Contains databases and archives
```

---

## Data Format Requirements

### JSON Structure

EdgeLake expects **JSON arrays** or **single JSON objects**:

**Array format** (preferred for bulk loading):
```json
[
  {"timestamp": "2025-01-05T10:00:00Z", "device": "dev1", "value": 42.5},
  {"timestamp": "2025-01-05T10:01:00Z", "device": "dev2", "value": 38.1},
  {"timestamp": "2025-01-05T10:02:00Z", "device": "dev1", "value": 43.2}
]
```

**Single object**:
```json
{
  "timestamp": "2025-01-05T10:00:00Z",
  "device": "sensor_001",
  "temperature": 72.5,
  "humidity": 45.2
}
```

### Required Fields

**Minimum required**:
- At least one field (EdgeLake creates columns dynamically)

**Recommended**:
- **timestamp** field (for time-series queries)
  - Format: ISO 8601 (`YYYY-MM-DDTHH:MM:SSZ`)
  - Or Unix epoch (milliseconds)

**Auto-added fields** by EdgeLake:
- `insert_timestamp` - When data was inserted (default partition column)
- `tsd_id` - Internal time-series ID
- `tsd_name` - Source identifier

### Schema Auto-Creation

**EdgeLake automatically creates tables** if they don't exist:

```json
{
  "timestamp": "2025-01-05T10:00:00Z",
  "device_name": "sensor_001",
  "temperature": 72.5,
  "humidity": 45.2,
  "status": "online"
}
```

**Results in table**:
```sql
CREATE TABLE sensor_data (
    timestamp TIMESTAMP,
    device_name VARCHAR,
    temperature FLOAT,
    humidity FLOAT,
    status VARCHAR,
    insert_timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    tsd_id INT,
    tsd_name VARCHAR
);
```

**Data types inferred**:
- Numbers with decimals → `FLOAT`
- Whole numbers → `INT`
- Strings → `VARCHAR`
- ISO timestamps → `TIMESTAMP`
- Booleans → `BOOLEAN`

### Controlling Schema

**Option 1: Pre-create table** (recommended for production):

```bash
AL > sql my_database CREATE TABLE sensor_data (
    timestamp TIMESTAMP,
    device_name VARCHAR(50),
    temperature FLOAT,
    humidity FLOAT,
    status VARCHAR(20),
    insert_timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)
```

**Option 2: Use a table policy** (network-wide schema):

```bash
AL > blockchain get table where dbms=my_database and table=sensor_data
```

If no policy exists, create one via the Master node (covered in separate guide).

---

## Loading Existing Data

### Scenario: Migrate from CSV/SQL Dump

**Step 1: Convert to JSON**

```python
import pandas as pd

# Read CSV
df = pd.read_csv('sensor_data.csv')

# Convert to JSON (records format)
df.to_json('sensor_data.json', orient='records', date_format='iso')
```

**Step 2: Copy to Watch Directory**

```bash
docker cp sensor_data.json edgelake-operator:/app/EdgeLake/anylog/watch/my_database.sensor_data.001.json
```

**Step 3: Verify Ingestion**

```bash
AL > sql my_database SELECT COUNT(*) FROM sensor_data
```

### Scenario: Bulk Load from S3/Cloud Storage

```bash
# Download files from S3
aws s3 sync s3://my-bucket/sensor-data/ ./data-files/

# Copy to watch directory in batch
for file in data-files/*.json; do
  docker cp "$file" edgelake-operator:/app/EdgeLake/anylog/watch/
done
```

---

## Production Best Practices

### 1. Use REST PUT for Real-Time Data

```bash
# High-frequency data ingestion
curl -X PUT http://operator:32149 \
  -H "type: json" \
  -H "dbms: iot_data" \
  -H "table: sensor_readings" \
  -H "Content-Type: application/json" \
  -d @realtime_data.json
```

**Advantages**:
- Immediate feedback (success/failure)
- Lower latency
- Easy integration with applications

### 2. Use Watch Directory for Batch Loading

```bash
# Nightly batch job
0 2 * * * /opt/scripts/export_to_edgelake.sh
```

**Advantages**:
- Fire-and-forget (no need to wait for response)
- Handles large files efficiently
- Automatic retry on failure (file stays in error dir)

### 3. Use MQTT for IoT Streaming

**Advantages**:
- Designed for unreliable networks
- Low bandwidth overhead
- Pub/sub model scales well

### 4. Set Buffer Thresholds

Control when data is flushed to disk:

```bash
AL > set buffer threshold where time=60 seconds and volume=100KB and write_immediate=false
```

**Explanation**:
- Batch writes for better performance
- Flush every 60 seconds OR when buffer reaches 100KB
- `write_immediate=true` writes every record immediately (lower latency, higher I/O)

---

## Troubleshooting

### Issue: Files Not Being Processed from Watch Directory

**Symptoms**:
- Files stay in `!watch_dir`
- No errors in logs

**Diagnosis**:
```bash
# Check if streamer is running
AL > get processes

# Check watch directory
AL > get files in watch
```

**Fix**:
```bash
# Start streamer if not running
AL > run streamer
```

### Issue: Files Moving to Error Directory

**Symptoms**:
- Files appear in `/app/EdgeLake/data/error/`

**Diagnosis**:
```bash
# View error log
docker exec edgelake-operator tail -50 /app/EdgeLake/anylog/error.log

# Check file format
docker exec edgelake-operator cat /app/EdgeLake/data/error/failed_file.json
```

**Common Causes**:
1. **Invalid JSON syntax**
   ```json
   // BAD - trailing comma
   {"field": "value",}

   // GOOD
   {"field": "value"}
   ```

2. **Missing database/table name** (if not in filename)
   - Rename: `data.json` → `my_database.my_table.001.json`

3. **Schema mismatch** (wrong data type)
   - Check existing table schema: `AL > get columns where dbms=my_database and table=my_table`

### Issue: REST PUT Returns "Missing dbms name"

**Fix**: Add required headers:
```bash
curl -X PUT http://operator:32149 \
  -H "type: json" \
  -H "dbms: MY_DATABASE_NAME" \    # ← Add this
  -H "table: MY_TABLE_NAME" \       # ← Add this
  -H "Content-Type: application/json" \
  -d '{"data": "value"}'
```

### Issue: Data Not Appearing in Tables

**Diagnosis**:
```bash
# Check if database exists
AL > get databases

# Check if table exists
AL > get tables where dbms=my_database

# Check recent inserts
AL > sql my_database SELECT COUNT(*), MAX(insert_timestamp) FROM my_table
```

**Fix**:
- Verify `dbms` and `table` names match exactly (case-insensitive)
- Check logs for insertion errors: `docker exec edgelake-operator tail -f /app/EdgeLake/anylog/EdgeLake.log`

---

## Quick Reference Commands

### Check System Status

```bash
# Check all running processes
AL > get processes

# Check streamer specifically
AL > get streaming

# View databases
AL > get databases

# View tables in a database
AL > get tables where dbms=my_database

# View table schema
AL > get columns where dbms=my_database and table=my_table
```

### Monitor Data Flow

```bash
# Files in watch directory (waiting)
AL > get files in watch

# Files in prep directory (processing)
AL > get files in prep

# Archived files (successful)
AL > get files in archive

# Failed files (errors)
AL > get files in error
```

### Query Data

```bash
# Row count
AL > sql my_database SELECT COUNT(*) FROM my_table

# Recent data
AL > sql my_database SELECT * FROM my_table ORDER BY insert_timestamp DESC LIMIT 10

# Date range
AL > sql my_database SELECT * FROM my_table WHERE timestamp > NOW() - INTERVAL 1 HOUR
```

---

## Complete Loading Example

### Scenario: Load Historical Sensor Data

**Data**: 1 million sensor readings in CSV format

**Step 1: Convert CSV to JSON**

```python
import pandas as pd
import json

# Read large CSV
df = pd.read_csv('sensor_readings.csv')

# Split into chunks (100K rows each)
chunk_size = 100000
for i, chunk in enumerate(df.groupby(df.index // chunk_size)):
    chunk_data = chunk[1].to_dict('records')
    filename = f'iot_data.sensor_readings.chunk_{i:03d}.json'
    with open(filename, 'w') as f:
        json.dump(chunk_data, f)
```

**Step 2: Copy to Watch Directory**

```bash
# Copy all chunks
for file in iot_data.sensor_readings.chunk_*.json; do
  docker cp "$file" edgelake-operator:/app/EdgeLake/anylog/watch/
  sleep 5  # Give time to process each file
done
```

**Step 3: Monitor Progress**

```bash
# Watch log in real-time
docker exec edgelake-operator tail -f /app/EdgeLake/anylog/EdgeLake.log

# Check row count every 10 seconds
watch -n 10 'docker exec edgelake-operator python3 /app/EdgeLake/edge_lake/edgelake.py "sql iot_data SELECT COUNT(*) FROM sensor_readings"'
```

**Step 4: Verify**

```bash
AL > sql iot_data SELECT
    COUNT(*) as total_rows,
    MIN(timestamp) as earliest,
    MAX(timestamp) as latest,
    COUNT(DISTINCT device_name) as unique_devices
FROM sensor_readings
```

---

## Summary

### Choose Your Method

| Method | Use Case | Latency | Complexity | Best For |
|--------|----------|---------|------------|----------|
| **REST PUT** | Real-time data ingestion | Low (~ms) | Low | Applications, APIs, real-time dashboards |
| **Watch Directory** | Batch loading | Medium (~seconds) | Very Low | Data migration, scheduled imports, file dumps |
| **MQTT** | IoT streaming | Low (~ms) | Medium | Sensors, devices, event streams |
| **gRPC** | High-performance streaming | Very Low (~µs) | High | Critical systems, high throughput |

### Your Boss's Suggestion (Watch Directory)

**For loading a new table with existing data**:

1. **Format your data as JSON** (array of objects)
2. **Name file**: `<database_name>.<table_name>.<unique_id>.json`
3. **Copy to watch directory**: `docker cp data.json edgelake-operator:/app/EdgeLake/anylog/watch/`
4. **Wait ~5-10 seconds** (automatic processing)
5. **Verify**: `AL > sql <database> SELECT COUNT(*) FROM <table>`

**That's it!** EdgeLake handles the rest automatically.

---

**Document Version**: 1.0
**Last Updated**: 2025-01-05
**Author**: Claude Code (with user collaboration)
