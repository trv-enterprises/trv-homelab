#!/usr/bin/env python3
"""
Shelly Gen3 H&T MQTT collector for tsstore via Unix socket.

Subscribes to a sleepy Shelly sensor's MQTT status topics
(<prefix>/status/temperature:0, humidity:0, devicepower:0), buffers the
burst of messages the device publishes during a wake window, and writes
one combined record to tsstore when the burst settles.

Record fields (schema store): temp.c, humidity.pct, battery.pct,
battery.v, external_power (1/0). Fields missing from a burst are omitted
from the record — a schema store tolerates records that omit declared
fields.
"""

import json
import os
import socket
import sys
import threading
import time

import paho.mqtt.client as mqtt

# Configuration from environment (systemd unit supplies EnvironmentFile);
# no secrets in source.
MQTT_HOST = os.environ.get("MQTT_HOST", "127.0.0.1")
MQTT_PORT = int(os.environ.get("MQTT_PORT", "1883"))
TOPIC_PREFIX = os.environ.get("MQTT_TOPIC_PREFIX", "shelly/ht-kitchen")
SOCKET_PATH = os.environ.get("TSSTORE_SOCKET_PATH", "/var/run/tsstore/tsstore.sock")
STORE_NAME = os.environ.get("TSSTORE_STORE", "kitchen-env")
API_KEY = os.environ.get("TSSTORE_API_KEY")
# Seconds after the first message of a wake burst before the combined
# record is flushed. The device publishes temp/humidity/devicepower
# within a couple of seconds of waking.
FLUSH_DELAY = float(os.environ.get("FLUSH_DELAY", "5"))

if not API_KEY:
    sys.exit("TSSTORE_API_KEY env var is required")

buffer_lock = threading.Lock()
buffer = {}
burst_started = None


def tsstore_write(record):
    """Open socket, AUTH, write one record, QUIT. Per-burst connection —
    at one burst per ~5 minutes a persistent socket would sit idle."""
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    try:
        sock.connect(SOCKET_PATH)
        sock.settimeout(5.0)
        sock.send(f"AUTH {STORE_NAME} {API_KEY}\n".encode())
        resp = sock.recv(1024).decode().strip()
        if not resp.startswith("OK"):
            raise RuntimeError(f"auth failed: {resp}")
        sock.send((json.dumps(record) + "\n").encode())
        resp = sock.recv(1024).decode().strip()
        if not resp.startswith("OK"):
            raise RuntimeError(f"write failed: {resp}")
        sock.send(b"QUIT\n")
    finally:
        sock.close()


def on_connect(client, userdata, flags, rc):
    print(f"MQTT connected (rc={rc}), subscribing {TOPIC_PREFIX}/status/#")
    client.subscribe(f"{TOPIC_PREFIX}/status/#")


def on_message(client, userdata, msg):
    global burst_started
    try:
        payload = json.loads(msg.payload)
    except (ValueError, UnicodeDecodeError):
        return
    component = msg.topic.rsplit("/", 1)[-1]
    with buffer_lock:
        if burst_started is None:
            burst_started = time.time()
        if component == "temperature:0" and "tC" in payload:
            buffer["temp.c"] = round(float(payload["tC"]), 2)
        elif component == "humidity:0" and "rh" in payload:
            buffer["humidity.pct"] = round(float(payload["rh"]), 2)
        elif component == "devicepower:0":
            battery = payload.get("battery") or {}
            if "percent" in battery:
                buffer["battery.pct"] = int(battery["percent"])
            if "V" in battery:
                buffer["battery.v"] = round(float(battery["V"]), 2)
            external = payload.get("external") or {}
            if "present" in external:
                buffer["external_power"] = 1 if external["present"] else 0


def flusher():
    global burst_started
    while True:
        time.sleep(1)
        with buffer_lock:
            if burst_started is None or time.time() - burst_started < FLUSH_DELAY:
                continue
            record, buffer_copy = dict(buffer), None
            buffer.clear()
            burst_started = None
        if not record:
            continue
        try:
            tsstore_write(record)
            print(f"wrote: {record}")
        except Exception as e:
            print(f"tsstore write error: {e} (record dropped: {record})")


def main():
    print(f"Shelly H&T MQTT -> tsstore collector")
    print(f"MQTT: {MQTT_HOST}:{MQTT_PORT}  prefix: {TOPIC_PREFIX}")
    print(f"Store: {STORE_NAME}  socket: {SOCKET_PATH}")
    threading.Thread(target=flusher, daemon=True).start()
    client = mqtt.Client()
    client.on_connect = on_connect
    client.on_message = on_message
    client.connect(MQTT_HOST, MQTT_PORT, keepalive=60)
    # loop_forever handles reconnects with backoff
    client.loop_forever(retry_first_connection=True)


if __name__ == "__main__":
    main()
