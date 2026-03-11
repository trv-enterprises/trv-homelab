#!/usr/bin/env python3
"""
Load sample data into EdgeLake operator node.

This script PUTs JSON data to an EdgeLake operator node's REST endpoint using
the correct EdgeLake REST API format.

Tables are created automatically based on the data structure.

Usage:
    python3 load_sample_data_fixed.py --host <edgelake-lan-ip> --port 32149
"""

import argparse
import json
import requests
from datetime import datetime, timedelta
import sys


def generate_temperature_data(count=10):
    """Generate sample temperature sensor data."""
    base_time = datetime.utcnow()
    data = []

    locations = ["room1", "room2", "room3", "warehouse", "datacenter"]

    for i in range(count):
        timestamp = (base_time - timedelta(minutes=i)).isoformat() + "Z"

        data.append({
            "value": 20.0 + (i % 10) + (i * 0.1),
            "location": locations[i % len(locations)],
            "sensor_id": f"temp_sensor_{i % 3}",
            "timestamp": timestamp
        })

    return data


def generate_humidity_data(count=10):
    """Generate sample humidity sensor data."""
    base_time = datetime.utcnow()
    data = []

    locations = ["room1", "room2", "room3", "warehouse", "datacenter"]

    for i in range(count):
        timestamp = (base_time - timedelta(minutes=i)).isoformat() + "Z"

        data.append({
            "value": 40.0 + (i % 20) + (i * 0.5),
            "location": locations[i % len(locations)],
            "sensor_id": f"humid_sensor_{i % 3}",
            "timestamp": timestamp
        })

    return data


def generate_pressure_data(count=10):
    """Generate sample pressure sensor data."""
    base_time = datetime.utcnow()
    data = []

    locations = ["room1", "room2", "warehouse"]

    for i in range(count):
        timestamp = (base_time - timedelta(minutes=i)).isoformat() + "Z"

        data.append({
            "value": 1013.25 + (i % 5) - 2.5,
            "location": locations[i % len(locations)],
            "sensor_id": f"pressure_sensor_{i % 2}",
            "unit": "hPa",
            "timestamp": timestamp
        })

    return data


def put_data(host, port, dbms, table, data):
    """
    PUT data to EdgeLake operator node using correct REST API format.

    Args:
        host: Operator node hostname/IP
        port: REST port (default 32149)
        dbms: Database name
        table: Table name
        data: List of data records (without dbms/table fields)

    Returns:
        Response object

    Raises:
        requests.exceptions.RequestException on failure
    """
    url = f"http://{host}:{port}"

    # EdgeLake PUT requires these specific headers
    headers = {
        "type": "json",                    # Required: Data format
        "dbms": dbms,                      # Required: Database name
        "table": table,                    # Required: Table name
        "Content-Type": "application/json"
    }

    payload = json.dumps(data)

    print(f"Putting {len(data)} records to {url}")
    print(f"  Database: {dbms}")
    print(f"  Table: {table}")
    print(f"  Payload size: {len(payload)} bytes")

    try:
        response = requests.put(url, headers=headers, data=payload, timeout=30)
        response.raise_for_status()

        print(f"  ✓ Success! Status code: {response.status_code}")
        if response.text:
            # Parse response to show hash
            try:
                resp_json = json.loads(response.text)
                if "AnyLog.hash" in resp_json:
                    print(f"  Hash: {resp_json['AnyLog.hash'][:16]}...")
            except:
                print(f"  Response: {response.text}")

        return response

    except requests.exceptions.RequestException as e:
        print(f"  ✗ Error posting data: {e}", file=sys.stderr)
        if hasattr(e, 'response') and e.response is not None:
            print(f"  Response status: {e.response.status_code}", file=sys.stderr)
            print(f"  Response body: {e.response.text}", file=sys.stderr)
        raise


def main():
    parser = argparse.ArgumentParser(
        description="Load sample data into EdgeLake operator node"
    )
    parser.add_argument(
        "--host",
        default="localhost",
        help="Operator node hostname/IP (default: localhost)"
    )
    parser.add_argument(
        "--port",
        type=int,
        default=32149,
        help="REST port (default: 32149)"
    )
    parser.add_argument(
        "--dbms",
        default="aiops",
        help="Database name (default: aiops)"
    )
    parser.add_argument(
        "--count",
        type=int,
        default=10,
        help="Number of records per table (default: 10)"
    )
    parser.add_argument(
        "--tables",
        nargs="+",
        default=["temperature", "humidity", "pressure"],
        choices=["temperature", "humidity", "pressure", "all"],
        help="Tables to populate (default: all)"
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show data without posting"
    )

    args = parser.parse_args()

    # Handle "all" option
    if "all" in args.tables:
        args.tables = ["temperature", "humidity", "pressure"]

    print(f"EdgeLake Data Loader (Fixed)")
    print(f"=" * 60)
    print(f"Target: http://{args.host}:{args.port}")
    print(f"Database: {args.dbms}")
    print(f"Tables: {', '.join(args.tables)}")
    print(f"Records per table: {args.count}")
    print(f"=" * 60)
    print()

    # Generate data for selected tables
    datasets = {}

    if "temperature" in args.tables:
        print("Generating temperature data...")
        datasets["temperature"] = generate_temperature_data(args.count)
        print(f"  Generated {len(datasets['temperature'])} temperature records")

    if "humidity" in args.tables:
        print("Generating humidity data...")
        datasets["humidity"] = generate_humidity_data(args.count)
        print(f"  Generated {len(datasets['humidity'])} humidity records")

    if "pressure" in args.tables:
        print("Generating pressure data...")
        datasets["pressure"] = generate_pressure_data(args.count)
        print(f"  Generated {len(datasets['pressure'])} pressure records")

    print()

    if args.dry_run:
        print("Dry run mode - showing sample data:")
        for table_name, data in datasets.items():
            print(f"\n{table_name} table (first record):")
            print(json.dumps(data[0], indent=2))
        print(f"\nTotal: {sum(len(d) for d in datasets.values())} records across {len(datasets)} tables")
        return

    # Post data (separate PUT for each table)
    success_count = 0
    failed_tables = []

    for table_name, data in datasets.items():
        print(f"Loading {table_name} table...")
        try:
            response = put_data(args.host, args.port, args.dbms, table_name, data)
            success_count += 1
        except Exception as e:
            print(f"  Failed to load {table_name}: {e}", file=sys.stderr)
            failed_tables.append(table_name)

    print()
    print("=" * 60)
    if success_count == len(datasets):
        print("✓ All data loaded successfully!")
    else:
        print(f"⚠ Partial success: {success_count}/{len(datasets)} tables loaded")
        if failed_tables:
            print(f"Failed tables: {', '.join(failed_tables)}")

    print()
    print("Verify data:")
    print(f"  1. Connect to operator: docker exec -it <operator-container> python3 /app/EdgeLake/edge_lake/edgelake.py")
    print(f"  2. List tables: AL > get tables where dbms = {args.dbms}")
    print(f"  3. Query data: AL > sql {args.dbms} SELECT * FROM temperature LIMIT 10")

    if failed_tables:
        sys.exit(1)


if __name__ == "__main__":
    main()
