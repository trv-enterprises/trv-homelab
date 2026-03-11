#!/usr/bin/env python3
"""
Load sample data into EdgeLake operator node.

This script POSTs JSON data to an EdgeLake operator node's REST endpoint.
Tables are created automatically based on the data structure.

Usage:
    python3 load_sample_data.py --host <edgelake-lan-ip> --port 32159
"""

import argparse
import json
import requests
from datetime import datetime, timedelta
import sys


def generate_temperature_data(count=10, dbms="aiops"):
    """Generate sample temperature sensor data."""
    base_time = datetime.utcnow()
    data = []

    locations = ["room1", "room2", "room3", "warehouse", "datacenter"]

    for i in range(count):
        timestamp = (base_time - timedelta(minutes=i)).isoformat() + "Z"

        data.append({
            "dbms": dbms,
            "table": "temperature",
            "value": 20.0 + (i % 10) + (i * 0.1),
            "location": locations[i % len(locations)],
            "sensor_id": f"temp_sensor_{i % 3}",
            "timestamp": timestamp
        })

    return data


def generate_humidity_data(count=10, dbms="aiops"):
    """Generate sample humidity sensor data."""
    base_time = datetime.utcnow()
    data = []

    locations = ["room1", "room2", "room3", "warehouse", "datacenter"]

    for i in range(count):
        timestamp = (base_time - timedelta(minutes=i)).isoformat() + "Z"

        data.append({
            "dbms": dbms,
            "table": "humidity",
            "value": 40.0 + (i % 20) + (i * 0.5),
            "location": locations[i % len(locations)],
            "sensor_id": f"humid_sensor_{i % 3}",
            "timestamp": timestamp
        })

    return data


def generate_pressure_data(count=10, dbms="aiops"):
    """Generate sample pressure sensor data."""
    base_time = datetime.utcnow()
    data = []

    locations = ["room1", "room2", "warehouse"]

    for i in range(count):
        timestamp = (base_time - timedelta(minutes=i)).isoformat() + "Z"

        data.append({
            "dbms": dbms,
            "table": "pressure",
            "value": 1013.25 + (i % 5) - 2.5,
            "location": locations[i % len(locations)],
            "sensor_id": f"pressure_sensor_{i % 2}",
            "unit": "hPa",
            "timestamp": timestamp
        })

    return data


def post_data(host, port, data, topic="sample_data"):
    """
    POST data to EdgeLake operator node.

    Args:
        host: Operator node hostname/IP
        port: REST port (default 32549)
        data: List of data records
        topic: Topic name for routing

    Returns:
        Response object
    """
    url = f"http://{host}:{port}"

    headers = {
        "command": "data",
        "topic": topic,
        "User-Agent": "AnyLog/1.23",
        "Content-Type": "text/plain"
    }

    payload = json.dumps(data)

    print(f"Posting {len(data)} records to {url}")
    print(f"Topic: {topic}")
    print(f"Payload size: {len(payload)} bytes")

    try:
        response = requests.post(url, headers=headers, data=payload, timeout=30)
        response.raise_for_status()

        print(f"Success! Status code: {response.status_code}")
        if response.text:
            print(f"Response: {response.text}")

        return response

    except requests.exceptions.RequestException as e:
        print(f"Error posting data: {e}", file=sys.stderr)
        if hasattr(e, 'response') and e.response is not None:
            print(f"Response status: {e.response.status_code}", file=sys.stderr)
            print(f"Response body: {e.response.text}", file=sys.stderr)
        raise


def main():
    parser = argparse.ArgumentParser(
        description="Load sample data into EdgeLake operator node"
    )
    parser.add_argument(
        "--host",
        default="<edgelake-lan-ip>",
        help="Operator node hostname/IP (default: <edgelake-lan-ip>)"
    )
    parser.add_argument(
        "--port",
        type=int,
        default=32159,
        help="REST port (default: 32159)"
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

    print(f"EdgeLake Data Loader")
    print(f"=" * 60)
    print(f"Target: http://{args.host}:{args.port}")
    print(f"Database: {args.dbms}")
    print(f"Tables: {', '.join(args.tables)}")
    print(f"Records per table: {args.count}")
    print(f"=" * 60)
    print()

    # Generate data for selected tables
    all_data = []

    if "temperature" in args.tables:
        print("Generating temperature data...")
        temp_data = generate_temperature_data(args.count, args.dbms)
        all_data.extend(temp_data)
        print(f"  Generated {len(temp_data)} temperature records")

    if "humidity" in args.tables:
        print("Generating humidity data...")
        humid_data = generate_humidity_data(args.count, args.dbms)
        all_data.extend(humid_data)
        print(f"  Generated {len(humid_data)} humidity records")

    if "pressure" in args.tables:
        print("Generating pressure data...")
        pressure_data = generate_pressure_data(args.count, args.dbms)
        all_data.extend(pressure_data)
        print(f"  Generated {len(pressure_data)} pressure records")

    print()

    if args.dry_run:
        print("Dry run mode - showing sample data:")
        print(json.dumps(all_data[:3], indent=2))
        print(f"... ({len(all_data)} total records)")
        return

    # Post data
    try:
        response = post_data(args.host, args.port, all_data)
        print()
        print("Data loaded successfully!")
        print()
        print("You can now query the data:")
        print(f"  - List tables: blockchain get table where dbms = {args.dbms}")
        print(f"  - Query data: sql {args.dbms} \"SELECT * FROM temperature LIMIT 10\"")

    except Exception as e:
        print(f"\nFailed to load data: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
