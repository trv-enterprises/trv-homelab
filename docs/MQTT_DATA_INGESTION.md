# EdgeLake MQTT Data Ingestion

How EdgeLake supports MQTT input to operator nodes for database ingestion.

## Data Flow

```
MQTT Broker (external or local)
        |
        v
  mqtt_client.py  ->  on_message() callback
        |
        v
  process_message()  ->  parse JSON payload
        |
        |-- process_using_bring()    (simple field extraction)
        |-- process_using_policies() (complex transformations)
        |
        v
  streaming_data.add_data()  ->  buffer data
        |
        v
  Write to watch_dir  ->  Operator ingests into SQLite/PostgreSQL
```

## Key Command: `run msg client`

This is the primary command to subscribe an operator node to MQTT data:

```sql
run msg client where broker = "mqtt.eclipse.org" and port = 1883 and
    topic = (name = "sensor/data" and
             dbms = "iot_db" and
             table = "readings" and
             column.timestamp.timestamp = "bring [ts]" and
             column.value.float = "bring [sensor_value]")
```

This tells EdgeLake:
- **Connect** to the MQTT broker
- **Subscribe** to the topic `sensor/data`
- **Map** incoming JSON fields to database columns using `bring [field_name]` syntax
- **Insert** into `iot_db.readings`

## Three Mapping Approaches

### 1. BRING Commands (Inline)

Directly extract JSON fields in the `run msg client` command:

```sql
run msg client where broker = "local" and
    topic = (name = "sensor/#" and
             dbms = "iot_db" and
             table = "readings" and
             column.timestamp.timestamp = "bring [ts]" and
             column.device.str = "bring [device_id]" and
             column.value.float = "bring [reading]")
```

### 2. Mapping Policies

Schema-based transformations stored in the blockchain/metadata:

```json
{
  "mapping": {
    "id": "policy_id",
    "schema": {
      "timestamp": {"type": "timestamp", "bring": "[ts]"},
      "sensor": {"type": "string", "bring": "[device_id]"},
      "value": {"type": "float", "bring": "[reading]"}
    }
  }
}
```

Usage:

```sql
run msg client where broker = "mqtt.eclipse.org" and
    topic = (name = "sensor/data" and policy = "policy_id")
```

### 3. Transform Policies

Complex transformations with regex patterns (useful for PLCs):

```json
{
  "transform": {
    "id": "plc_transform",
    "dbms": "[dbms_name]",
    "table": "[table_name]",
    "re_match": "pattern",
    "column": "[column_from_pattern]"
  }
}
```

## Broker Types Supported

| Broker Type | Description |
|---|---|
| External MQTT | Any standard broker (Mosquitto, AWS IoT, Google Cloud IoT) |
| `"local"` | EdgeLake's built-in Message Server (runs on the node itself) |
| `"rest"` | Routes HTTP POST data through the MQTT processing pipeline |
| `"kafka"` | Kafka consumer integration |

### Local Broker

The local broker option (`broker = "local"`) lets external devices publish MQTT directly to the operator node without needing a separate broker like Mosquitto. Requires the Message Server to be running on the node.

### REST Integration

Run an MQTT client with `broker = "rest"` and a custom `user-agent` header. Then POST data to the REST endpoint with that User-Agent header, and EdgeLake routes it through the MQTT processing pipeline.

```sql
run msg client where broker = "rest" and user-agent = "my_sensor" and
    topic = (name = "sensor/data" and dbms = "iot_db" and table = "readings" and
             column.timestamp.timestamp = "bring [ts]" and
             column.value.float = "bring [value]")
```

## Buffering and Thresholds

Data is not inserted row-by-row. `streaming_data.py` buffers messages and flushes to the database based on:

- **Time threshold**: default 60 seconds
- **Volume threshold**: default 10,000 bytes

This batching improves write performance for high-frequency IoT data.

Thresholds can be configured per table.

## Management Commands

```sql
get msg client              -- Show all MQTT clients and status
get msg client <id>         -- Details for specific client
get msg client detailed     -- Detailed information for all clients
set mqtt debug on           -- Print incoming messages for debugging
set mqtt debug off          -- Disable debug output
exit mqtt <id>              -- Stop a specific client
exit mqtt                   -- Stop all clients
```

## Error Handling and Statistics

Each MQTT subscription tracks:
- `message_counter` - total messages received
- `error_counter` - messages with processing errors
- Per-topic success/error counts and timestamps
- Optional `log_error` flag to write failed messages to an error directory

## Relevant Configuration in trv-edgelake-infra

In deployment configs (`configs/base_configs.env`):

- **`ANYLOG_BROKER_PORT`** - port for the local message broker (e.g., 32150 on operator2)
- `run msg client` commands are typically part of operator startup scripts or issued via the EdgeLake CLI after the node is running

## Key Source Files

| File | Purpose |
|---|---|
| `edge_lake/tcpip/mqtt_client.py` | MQTT client implementation, message processing, policies |
| `edge_lake/tcpip/message_server.py` | Local message broker, MQTT protocol handler |
| `edge_lake/tcpip/http_server.py` | REST API, HTTP integration with MQTT routing |
| `edge_lake/generic/streaming_data.py` | Buffered data management and threshold handling |
| `edge_lake/json_to_sql/mapping_policy.py` | Policy-based data transformation and mapping |
| `edge_lake/cmd/member_cmd.py` | Command parser and executor (includes MQTT commands) |
