#!/usr/bin/env python3
"""
Shelly device power monitor for tsstore via Unix socket.
Polls Shelly Gen2 devices (Plug, PM, etc.) for power metrics.

Collects: apower (watts), voltage, current, frequency, temperature, energy totals.
"""

import socket
import json
import time
import sys
import urllib.request
import urllib.error
import os

# Configuration - override via environment or edit defaults
SOCKET_PATH = os.environ.get("TSSTORE_SOCKET", "/home/<user>/run/tsstore.sock")
STORE_NAME = os.environ.get("TSSTORE_STORE", "shelly-plug")
API_KEY = os.environ.get("TSSTORE_API_KEY", "CHANGEME")
SAMPLE_INTERVAL = int(os.environ.get("SAMPLE_INTERVAL", "10"))

# Shelly device settings
SHELLY_HOST = os.environ.get("SHELLY_HOST", "<cam-driveway-lan-ip>0")
SHELLY_SWITCH_ID = int(os.environ.get("SHELLY_SWITCH_ID", "0"))


def fetch_shelly_status(host, switch_id):
    """Fetch power status from Shelly device via HTTP RPC."""
    url = f"http://{host}/rpc/Switch.GetStatus?id={switch_id}"
    try:
        with urllib.request.urlopen(url, timeout=5) as response:
            return json.loads(response.read().decode())
    except urllib.error.URLError as e:
        raise Exception(f"Failed to reach Shelly at {host}: {e}")
    except json.JSONDecodeError as e:
        raise Exception(f"Invalid JSON from Shelly: {e}")


def connect_and_auth(socket_path, store_name, api_key):
    """Connect to tsstore Unix socket and authenticate."""
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.connect(socket_path)
    sock.settimeout(5.0)

    auth_cmd = f"AUTH {store_name} {api_key}\n"
    sock.send(auth_cmd.encode())

    response = sock.recv(1024).decode().strip()
    if not response.startswith("OK"):
        raise Exception(f"Authentication failed: {response}")

    return sock


def send_reading(sock, data):
    """Send a single reading to tsstore."""
    line = json.dumps(data) + "\n"
    sock.send(line.encode())

    response = sock.recv(1024).decode().strip()
    if not response.startswith("OK"):
        raise Exception(f"Write failed: {response}")

    parts = response.split()
    return int(parts[1]) if len(parts) >= 2 else None


def extract_power_data(status):
    """Extract power metrics from Shelly status response."""
    data = {
        "output": status.get("output", False),
        "apower": status.get("apower", 0.0),       # Active power in watts
        "voltage": status.get("voltage", 0.0),     # Voltage in V
        "current": status.get("current", 0.0),     # Current in A
        "freq": status.get("freq", 0.0),           # Frequency in Hz
    }

    # Temperature (may be nested)
    temp = status.get("temperature", {})
    if isinstance(temp, dict):
        data["temp_c"] = temp.get("tC", 0.0)
    else:
        data["temp_c"] = 0.0

    # Energy totals (forward)
    aenergy = status.get("aenergy", {})
    if isinstance(aenergy, dict):
        data["energy_total_wh"] = aenergy.get("total", 0.0)

    # Return energy (for bidirectional monitoring, if present)
    ret_aenergy = status.get("ret_aenergy", {})
    if isinstance(ret_aenergy, dict) and ret_aenergy.get("total", 0.0) > 0:
        data["ret_energy_total_wh"] = ret_aenergy.get("total", 0.0)

    return data


def main():
    print(f"Shelly power monitor for tsstore")
    print(f"Shelly host: {SHELLY_HOST} (switch {SHELLY_SWITCH_ID})")
    print(f"Socket: {SOCKET_PATH}")
    print(f"Store: {STORE_NAME}")
    print(f"Sample interval: {SAMPLE_INTERVAL}s")
    print()

    sock = None
    reconnect_delay = 1
    sample_count = 0

    while True:
        try:
            if sock is None:
                print("Connecting to tsstore...")
                sock = connect_and_auth(SOCKET_PATH, STORE_NAME, API_KEY)
                print("Connected!")
                reconnect_delay = 1

            # Fetch from Shelly
            status = fetch_shelly_status(SHELLY_HOST, SHELLY_SWITCH_ID)
            data = extract_power_data(status)
            ts = send_reading(sock, data)
            sample_count += 1

            on_str = "ON" if data["output"] else "OFF"
            print(f"[{sample_count}] {on_str} {data['apower']:.1f}W "
                  f"{data['voltage']:.1f}V {data['current']:.3f}A "
                  f"{data['temp_c']:.1f}C total:{data.get('energy_total_wh', 0):.1f}Wh")

            time.sleep(SAMPLE_INTERVAL)

        except KeyboardInterrupt:
            print("\nShutting down...")
            if sock:
                try:
                    sock.send(b"QUIT\n")
                    sock.close()
                except:
                    pass
            sys.exit(0)

        except Exception as e:
            print(f"Error: {e}")
            if sock:
                try:
                    sock.close()
                except:
                    pass
                sock = None

            print(f"Reconnecting in {reconnect_delay}s...")
            time.sleep(reconnect_delay)
            reconnect_delay = min(reconnect_delay * 2, 60)


if __name__ == "__main__":
    main()
