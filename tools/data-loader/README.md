# EdgeLake Table Loader

This utility loads sample data into an EdgeLake operator node for testing purposes.

## Prerequisites

- Python 3.x with `requests` library
- An EdgeLake **operator node** running with REST endpoint enabled
- The operator node must have a database connected (e.g., PostgreSQL, SQLite)

## Installation

```bash
pip install requests
```

## Usage

### Basic Usage (Default Settings)

Load sample data to operator at `<edgelake-lan-ip>:32549`:

```bash
python3 load_sample_data.py
```

This will create three tables in the `aiops` database:
- `temperature` (10 records)
- `humidity` (10 records)
- `pressure` (10 records)

### Custom Operator Node

```bash
python3 load_sample_data.py --host <cam-porch-lan-ip> --port 32549
```

### Custom Database Name

```bash
python3 load_sample_data.py --dbms mydb
```

### Load Specific Tables

```bash
# Load only temperature data
python3 load_sample_data.py --tables temperature

# Load temperature and humidity
python3 load_sample_data.py --tables temperature humidity
```

### More Records

```bash
# Load 100 records per table
python3 load_sample_data.py --count 100
```

### Dry Run (Preview Data)

```bash
python3 load_sample_data.py --dry-run
```

## Data Structure

### Temperature Table
```json
{
  "dbms": "aiops",
  "table": "temperature",
  "value": 23.5,
  "location": "room1",
  "sensor_id": "temp_sensor_0",
  "timestamp": "2025-10-30T01:00:00.000Z"
}
```

### Humidity Table
```json
{
  "dbms": "aiops",
  "table": "humidity",
  "value": 45.2,
  "location": "warehouse",
  "sensor_id": "humid_sensor_1",
  "timestamp": "2025-10-30T01:00:00.000Z"
}
```

### Pressure Table
```json
{
  "dbms": "aiops",
  "table": "pressure",
  "value": 1013.25,
  "location": "datacenter",
  "sensor_id": "pressure_sensor_0",
  "unit": "hPa",
  "timestamp": "2025-10-30T01:00:00.000Z"
}
```

## Verifying Data

After loading data, you can verify it was ingested:

### Via EdgeLake CLI

```bash
# Connect to operator node
docker exec -it <operator-container> bash

# Inside container
python3 edge_lake/edgelake.py

# Check tables exist
AL > blockchain get table where dbms = aiops

# Query data
AL > sql aiops "SELECT * FROM temperature LIMIT 10"
```

### Via MCP Tools (Query Node)

If you have a query node with MCP server running:

```python
# list_tables tool
{
  "database": "aiops"
}

# query tool
{
  "database": "aiops",
  "table": "temperature",
  "limit": 10
}
```

## Troubleshooting

### "Connection refused"
- Verify the operator node is running
- Check the REST port is correct (default: 32549)
- Ensure firewall allows connections to REST port

### "Failed to load data"
- Ensure operator has a connected database
- Check operator logs for errors
- Verify the database name exists

### Tables not appearing in blockchain
- Operator may need time to publish metadata
- Check operator's blockchain sync status
- Verify operator is configured to publish to blockchain/master

## Architecture Note

**Important**: This script posts data to an **operator node**, not a query node.

- **Operator nodes** ingest data and store it locally
- **Query nodes** only orchestrate queries across operators
- Tables are created automatically on first data insertion
- Table metadata is published to the blockchain after creation
