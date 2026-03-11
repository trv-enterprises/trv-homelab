# Data Loader Validation Report

**Date**: 2025-01-05
**Script**: `load_sample_data.py`
**Status**: ❌ **CRITICAL ISSUES FOUND** - Script will not work with EdgeLake

---

## Executive Summary

The current `load_sample_data.py` script uses the **legacy AnyLog POST API format**, which is incompatible with EdgeLake's REST PUT API. The script will fail to load data and return errors.

**Issues Found**: 4 critical, 2 minor
**Corrected Version**: `load_sample_data_fixed.py` (created)

---

## Critical Issues

### 1. Wrong HTTP Method ❌

**Location**: Line 115

**Current Code**:
```python
response = requests.post(url, headers=headers, data=payload, timeout=30)
```

**Problem**: EdgeLake REST API requires `PUT` method for data ingestion, not `POST`.

**Evidence**: `edge_lake/tcpip/http_server.py:1411`
```python
def do_PUT(self):
    # EdgeLake data ingestion endpoint
```

**Impact**: Request will fail with 405 Method Not Allowed or be routed to wrong handler

**Fix**:
```python
response = requests.put(url, headers=headers, data=payload, timeout=30)
```

---

### 2. Incorrect HTTP Headers ❌

**Location**: Lines 101-106

**Current Code**:
```python
headers = {
    "command": "data",
    "topic": topic,
    "User-Agent": "AnyLog/1.23",
    "Content-Type": "text/plain"
}
```

**Problem**: Headers use legacy AnyLog format. EdgeLake PUT endpoint requires specific headers.

**Evidence**: `edge_lake/tcpip/http_server.py:2172-2183`
```python
db_name = get_value_from_headers(al_headers, "dbms")
if not db_name:
    ret_val = process_status.Missing_dbms_name
    status.keep_error("Missing 'dbms' name in REST PUT command")

tb_name = get_value_from_headers(al_headers, "table")
if not tb_name:
    ret_val = process_status.ERR_table_name
    status.keep_error("Missing 'table' name in REST PUT command")
```

**Impact**:
- Missing `dbms` → Error: "Missing 'dbms' name in REST PUT command"
- Missing `table` → Error: "Missing 'table' name in REST PUT command"
- Missing `type` → May default incorrectly

**Fix**:
```python
headers = {
    "type": "json",                    # Required
    "dbms": dbms_name,                 # Required
    "table": table_name,               # Required
    "Content-Type": "application/json"
}
```

---

### 3. Wrong Data Format ❌

**Location**: Lines 29-36, 51-58, 73-81

**Current Code**:
```python
data.append({
    "dbms": dbms,              # ❌ Should be in headers
    "table": "temperature",    # ❌ Should be in headers
    "value": 20.0,
    "location": "room1",
    "sensor_id": "temp_sensor_0",
    "timestamp": timestamp
})
```

**Problem**:
- `dbms` and `table` fields should be in HTTP headers, NOT in data payload
- Including them in data will create unnecessary table columns

**Evidence**: EdgeLake extracts dbms/table from headers (shown above), then inserts the payload data directly into those tables.

**Impact**:
- Creates extra columns: `dbms` and `table` in every table
- Violates data normalization
- Wastes storage space

**Fix**:
```python
data.append({
    "value": 20.0,
    "location": "room1",
    "sensor_id": "temp_sensor_0",
    "timestamp": timestamp
})
```

Headers specify the target:
```python
headers = {"dbms": "aiops", "table": "temperature", ...}
```

---

### 4. Multiple Tables in Single Request ❌

**Location**: Lines 187-205

**Current Code**:
```python
all_data = []

if "temperature" in args.tables:
    temp_data = generate_temperature_data(...)
    all_data.extend(temp_data)  # ❌ Mixing tables

if "humidity" in args.tables:
    humid_data = generate_humidity_data(...)
    all_data.extend(humid_data)  # ❌ Mixing tables

# Single POST with mixed table data
response = post_data(args.host, args.port, all_data)
```

**Problem**: EdgeLake PUT endpoint accepts only ONE table per request (headers specify single `dbms` and `table`)

**Evidence**: `http_server.py:1479-1480`
```python
ret_val, dbms_name, table_name, source, instructions, file_type = put_params_from_header(...)
# Single dbms_name and table_name extracted from headers
```

**Impact**: All data will go to ONE table (likely the one specified in headers), not distributed across temperature/humidity/pressure tables

**Fix**: Make separate PUT requests for each table
```python
for table_name, data in datasets.items():
    put_data(host, port, dbms, table_name, data)
```

---

## Minor Issues

### 5. Wrong Content-Type 🟡

**Current**: `"Content-Type": "text/plain"`
**Expected**: `"Content-Type": "application/json"`

**Impact**: EdgeLake may still process it, but violates HTTP standards for JSON payloads

---

### 6. Missing Per-Table Error Handling 🟡

**Current**: Script exits on first table failure

**Better**: Continue loading other tables, report summary at end

**Fix**: Implemented in `load_sample_data_fixed.py`

---

## Testing Comparison

### Original Script (BROKEN)

```bash
python3 load_sample_data.py --host localhost --port 32149
```

**Expected Result**: ❌ Failure
```
Error posting data: 400 Client Error: Bad Request
Response: {"AnyLog.error on REST server":"Missing 'dbms' name in REST PUT command"}
```

**OR** (if POST is accepted):
```
Error: HTTP method POST is not allowed on this endpoint
```

### Fixed Script (WORKING)

```bash
python3 load_sample_data_fixed.py --host localhost --port 32149
```

**Expected Result**: ✓ Success
```
EdgeLake Data Loader (Fixed)
============================================================
Target: http://localhost:32149
Database: aiops
Tables: temperature, humidity, pressure
Records per table: 10
============================================================

Loading temperature table...
  Putting 10 records to http://localhost:32149
  Database: aiops
  Table: temperature
  ✓ Success! Status code: 200
  Hash: abc123def456...

Loading humidity table...
  Putting 10 records to http://localhost:32149
  Database: aiops
  Table: humidity
  ✓ Success! Status code: 200
  Hash: def789ghi012...

Loading pressure table...
  Putting 10 records to http://localhost:32149
  Database: aiops
  Table: pressure
  ✓ Success! Status code: 200
  Hash: ghi345jkl678...

============================================================
✓ All data loaded successfully!
```

---

## EdgeLake REST API Reference

Based on `edge_lake/tcpip/http_server.py` analysis:

### PUT Endpoint for Data Ingestion

**URL**: `http://<operator-host>:<rest-port>/`

**Method**: `PUT`

**Required Headers**:
| Header | Type | Example | Description |
|--------|------|---------|-------------|
| `type` | string | `json` | Data format (json, sql, csv) |
| `dbms` | string | `aiops` | Database name (lowercase) |
| `table` | string | `temperature` | Table name (lowercase) |
| `Content-Type` | string | `application/json` | MIME type of payload |

**Optional Headers**:
| Header | Type | Example | Description |
|--------|------|---------|-------------|
| `source` | string | `mqtt` | Data source identifier |
| `instructions` | string | `policy_id` | Mapping policy ID |

**Request Body**: JSON array of records
```json
[
  {
    "timestamp": "2025-01-05T10:00:00Z",
    "value": 72.5,
    "location": "room1",
    "sensor_id": "sensor_001"
  },
  {
    "timestamp": "2025-01-05T10:01:00Z",
    "value": 68.3,
    "location": "room2",
    "sensor_id": "sensor_002"
  }
]
```

**Success Response** (200 OK):
```json
{
  "AnyLog.status": "Success",
  "AnyLog.hash": "abc123def456789..."
}
```

**Error Response** (500 Internal Server Error):
```json
{
  "AnyLog.error on REST server": "Missing 'dbms' name in REST PUT command"
}
```

---

## Recommendations

### Immediate Action Required

1. **Replace `load_sample_data.py` with `load_sample_data_fixed.py`**
   ```bash
   cd ../utilities/edgelake/data-loader/
   mv load_sample_data.py load_sample_data_legacy.py
   mv load_sample_data_fixed.py load_sample_data.py
   ```

2. **Update README.md** to reflect correct API usage

3. **Test fixed script**:
   ```bash
   # Ensure operator is running
   docker ps | grep operator

   # Test with dry-run
   python3 load_sample_data.py --dry-run

   # Load data
   python3 load_sample_data.py --host localhost --port 32149

   # Verify
   docker exec -it edgelake-operator python3 /app/EdgeLake/edge_lake/edgelake.py
   AL > get tables where dbms = aiops
   AL > sql aiops SELECT COUNT(*) FROM temperature
   ```

### Future Improvements

1. **Add retry logic** for transient network failures
2. **Batch size control** for very large datasets
3. **Progress bar** for long-running loads
4. **CSV/Excel input** support (convert to JSON internally)
5. **Validation mode** to check data before loading

---

## Technical Details

### HTTP Flow Comparison

**Original (Broken)**:
```
Client → POST http://operator:32149
Headers: {command: "data", topic: "sample"}
Body: [{"dbms": "aiops", "table": "temp", "value": 72.5}, ...]
         ↓
EdgeLake: 405 Method Not Allowed OR Missing dbms header
```

**Fixed (Working)**:
```
Client → PUT http://operator:32149
Headers: {type: "json", dbms: "aiops", table: "temperature"}
Body: [{"value": 72.5, "location": "room1", ...}, ...]
         ↓
EdgeLake do_PUT() → al_put() → put_params_from_header()
         ↓
streaming_data.add_data(dbms="aiops", table="temperature", msg_data=[...])
         ↓
Database: INSERT INTO aiops.temperature VALUES (...)
         ↓
Response: {"AnyLog.status": "Success", "AnyLog.hash": "..."}
```

### Code Path in EdgeLake

```
http_server.py:
  do_PUT() [line 1411]
    ↓
  al_put() [line 1444]
    ↓
  get_msg_body() [parses JSON]
    ↓
  put_params_from_header() [line 2164]
    - Extracts dbms from headers [line 2172]
    - Extracts table from headers [line 2178]
    ↓
  streaming_data.add_data() [line 1485]
    - Writes to !prep_dir
    - Streamer processes file
    - Inserts into database
    ↓
  Response: Success with hash [line 1499]
```

---

## Conclusion

The original `load_sample_data.py` script is **completely incompatible** with EdgeLake's REST API. It uses a legacy format that will result in errors.

**Action Required**: Use the corrected `load_sample_data_fixed.py` script immediately.

**Validation Status**:
- ❌ Original script: **FAILED**
- ✅ Fixed script: **VALIDATED** (code review against EdgeLake source)

**Next Steps**:
1. Replace original script
2. Test with live operator node
3. Update documentation

---

**Report Generated**: 2025-01-05
**Reviewed By**: Claude Code
**EdgeLake Version**: Based on current main branch (2025-01-05)
