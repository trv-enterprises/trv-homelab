#!/usr/bin/env python3
"""Caseta MQTT Bridge — bidirectional bridge between Lutron Caseta and MQTT.

Events (Caseta -> MQTT): Device state changes publish JSON to caseta/<device> topics.
Commands (MQTT -> Caseta): Subscribe to caseta/<device>/set to control devices.

Uses pylutron-caseta for LEAP protocol and paho-mqtt for the broker connection.
"""

import asyncio
import json
import logging
import os
import re
import signal
import sys
from datetime import datetime, timezone
from pathlib import Path

import paho.mqtt.client as mqtt
import yaml
from pylutron_caseta.smartbridge import Smartbridge

log = logging.getLogger("caseta-bridge")

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

DEFAULT_CONFIG_PATH = "/etc/caseta-bridge/config.yaml"


def load_config(path: str) -> dict:
    """Load and validate YAML config."""
    with open(path) as f:
        cfg = yaml.safe_load(f)

    # Ensure required sections exist
    for section in ("caseta", "mqtt"):
        if section not in cfg:
            raise ValueError(f"missing '{section}' section in config")

    caseta = cfg["caseta"]
    if not caseta.get("bridge_host") or caseta["bridge_host"] == "192.168.1.XXX":
        raise ValueError("caseta.bridge_host must be set to the L-BDG2 IP")

    cfg.setdefault("topic_prefix", "caseta")
    cfg.setdefault("device_names", {})
    cfg["mqtt"].setdefault("broker", "localhost")
    cfg["mqtt"].setdefault("port", 1883)
    cfg["mqtt"].setdefault("client_id", "caseta-bridge")
    cfg["caseta"].setdefault("cert_dir", "/etc/caseta-bridge/certs")

    return cfg


# ---------------------------------------------------------------------------
# Device name helpers
# ---------------------------------------------------------------------------

def sanitize_name(name: str) -> str:
    """Convert a device name to an MQTT-safe topic segment.

    Lowercase, spaces to underscores, strip anything that isn't
    alphanumeric or underscore.
    """
    name = name.lower().replace(" ", "_")
    return re.sub(r"[^a-z0-9_]", "", name)


def build_device_maps(devices: dict, name_overrides: dict | None) -> tuple[dict, dict]:
    """Build bidirectional device_id <-> topic_name mappings.

    Returns (id_to_name, name_to_id).
    """
    id_to_name: dict[str, str] = {}
    name_to_id: dict[str, str] = {}
    name_overrides = name_overrides or {}

    for device_id, device in devices.items():
        # name_overrides keys are strings (device IDs)
        if str(device_id) in name_overrides:
            name = sanitize_name(str(name_overrides[str(device_id)]))
        else:
            name = sanitize_name(device.get("name", f"device_{device_id}"))

        # Handle duplicates by appending device_id
        if name in name_to_id:
            name = f"{name}_{device_id}"

        id_to_name[str(device_id)] = name
        name_to_id[name] = str(device_id)

    return id_to_name, name_to_id


def device_type_label(device: dict) -> str:
    """Return a simple type string from a pylutron-caseta device dict."""
    dtype = device.get("type", "unknown")
    # pylutron-caseta uses strings like "WallDimmer", "Pico3ButtonRaiseLower", etc.
    dtype_lower = str(dtype).lower()
    if "dimmer" in dtype_lower or "switch" in dtype_lower:
        return "light"
    if "pico" in dtype_lower or "remote" in dtype_lower:
        return "remote"
    if "shade" in dtype_lower or "blind" in dtype_lower:
        return "shade"
    if "fan" in dtype_lower:
        return "fan"
    return "unknown"


# ---------------------------------------------------------------------------
# MQTT helpers
# ---------------------------------------------------------------------------

def create_mqtt_client(cfg: dict) -> mqtt.Client:
    """Create and configure a paho-mqtt v2 client."""
    client = mqtt.Client(
        callback_api_version=mqtt.CallbackAPIVersion.VERSION2,
        client_id=cfg["mqtt"]["client_id"],
    )
    client.enable_logger(log)
    client.reconnect_delay_set(min_delay=1, max_delay=30)
    return client


def publish_json(client: mqtt.Client, topic: str, payload: dict) -> None:
    """Publish a JSON payload to an MQTT topic."""
    client.publish(topic, json.dumps(payload), qos=1, retain=True)


# ---------------------------------------------------------------------------
# CasetaBridge
# ---------------------------------------------------------------------------

class CasetaBridge:
    """Bidirectional Caseta <-> MQTT bridge."""

    def __init__(self, config_path: str):
        self._config_path = config_path
        self._cfg = load_config(config_path)
        self._loop: asyncio.AbstractEventLoop | None = None

        # pylutron-caseta bridge handle
        self._bridge: Smartbridge | None = None

        # MQTT client
        self._mqtt = create_mqtt_client(self._cfg)

        # Device mappings (populated after connect)
        self._devices: dict = {}
        self._id_to_name: dict[str, str] = {}
        self._name_to_id: dict[str, str] = {}

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    async def start(self) -> None:
        """Connect to both Caseta bridge and MQTT broker, then run."""
        self._loop = asyncio.get_running_loop()

        # --- Caseta connection ---
        cert_dir = Path(self._cfg["caseta"]["cert_dir"])
        self._bridge = Smartbridge.create_tls(
            hostname=self._cfg["caseta"]["bridge_host"],
            keyfile=str(cert_dir / "client.key"),
            certfile=str(cert_dir / "client.crt"),
            ca_certs=str(cert_dir / "ca.crt"),
        )
        log.info("connecting to Caseta bridge at %s", self._cfg["caseta"]["bridge_host"])
        await self._bridge.connect()
        log.info("connected to Caseta bridge")

        # Discover devices and build name maps
        self._devices = self._bridge.get_devices()
        self._id_to_name, self._name_to_id = build_device_maps(
            self._devices, self._cfg.get("device_names", {})
        )
        log.info("discovered %d devices", len(self._devices))
        for did, name in self._id_to_name.items():
            dev = self._devices.get(did, self._devices.get(int(did), {}))
            log.info("  %s -> %s (%s)", did, name, device_type_label(dev))

        # Register Caseta subscribers for state changes
        # pylutron-caseta callbacks take zero args; fetch state via get_device()
        for device_id in self._devices:
            self._bridge.add_subscriber(
                str(device_id),
                lambda did=str(device_id): self._on_caseta_state(did),
            )

        # Register button subscribers for Pico remotes
        for device_id, device in self._devices.items():
            if device_type_label(device) == "remote":
                for button in device.get("buttons", []):
                    button_id = button.get("button_number") or button.get("number")
                    if button_id is not None:
                        self._bridge.add_button_subscriber(
                            str(button_id),
                            lambda event, did=str(device_id), bid=button_id: (
                                self._on_caseta_button(did, bid, event)
                            ),
                        )

        # --- MQTT connection ---
        prefix = self._cfg["topic_prefix"]
        self._mqtt.on_connect = self._on_mqtt_connect
        self._mqtt.on_message = self._on_mqtt_message

        broker = self._cfg["mqtt"]["broker"]
        port = self._cfg["mqtt"]["port"]
        log.info("connecting to MQTT broker at %s:%d", broker, port)
        self._mqtt.connect_async(broker, port)
        self._mqtt.loop_start()

        # Publish initial state for all devices
        await asyncio.sleep(1)  # brief pause to let MQTT connect
        await self._publish_all_states()

        log.info("bridge running — prefix=%s", prefix)

    async def stop(self) -> None:
        """Graceful shutdown."""
        log.info("shutting down")
        self._mqtt.loop_stop()
        self._mqtt.disconnect()
        if self._bridge:
            await self._bridge.close()
        log.info("shutdown complete")

    async def reload_config(self) -> None:
        """Reload config.yaml — refreshes device_names without reconnecting."""
        log.info("reloading config from %s", self._config_path)
        try:
            self._cfg = load_config(self._config_path)
            self._id_to_name, self._name_to_id = build_device_maps(
                self._devices, self._cfg.get("device_names", {})
            )
            log.info("config reloaded — %d device name mappings", len(self._id_to_name))
        except Exception:
            log.exception("config reload failed, keeping previous config")

    # ------------------------------------------------------------------
    # Caseta -> MQTT (events)
    # ------------------------------------------------------------------

    def _on_caseta_state(self, device_id: str) -> None:
        """Callback when a Caseta device state changes.

        pylutron-caseta callbacks take no args; fetch current state via get_device().
        """
        name = self._id_to_name.get(device_id, f"device_{device_id}")
        dev = self._bridge.get_device_by_id(device_id)
        if dev is None:
            dev = self._devices.get(device_id, self._devices.get(int(device_id), {}))
        prefix = self._cfg["topic_prefix"]
        level = dev.get("current_state", -1)

        payload = {
            "device_id": device_id,
            "name": dev.get("name", name),
            "type": device_type_label(dev),
            "state": "on" if level and level > 0 else "off",
            "level": level,
            "timestamp": datetime.now(timezone.utc).isoformat(),
        }

        topic = f"{prefix}/{name}"
        log.info("state: %s level=%s -> %s", name, level, topic)
        publish_json(self._mqtt, topic, payload)

    def _on_caseta_button(self, device_id: str, button_id: int, event: str) -> None:
        """Callback when a Pico remote button is pressed/released."""
        name = self._id_to_name.get(device_id, f"device_{device_id}")
        dev = self._devices.get(device_id, self._devices.get(int(device_id), {}))
        prefix = self._cfg["topic_prefix"]

        payload = {
            "device_id": device_id,
            "name": dev.get("name", name),
            "button": button_id,
            "action": event,
            "timestamp": datetime.now(timezone.utc).isoformat(),
        }

        topic = f"{prefix}/{name}/button"
        log.info("button: %s button=%s action=%s -> %s", name, button_id, event, topic)
        publish_json(self._mqtt, topic, payload)

    async def _publish_all_states(self) -> None:
        """Publish current state of all dimmable/switchable devices."""
        for device_id, device in self._devices.items():
            dtype = device_type_label(device)
            if dtype in ("light", "shade", "fan"):
                name = self._id_to_name.get(str(device_id), f"device_{device_id}")
                prefix = self._cfg["topic_prefix"]
                level = device.get("current_state", -1)

                payload = {
                    "device_id": str(device_id),
                    "name": device.get("name", name),
                    "type": dtype,
                    "state": "on" if level and level > 0 else "off",
                    "level": level,
                    "timestamp": datetime.now(timezone.utc).isoformat(),
                }
                publish_json(self._mqtt, f"{prefix}/{name}", payload)

    # ------------------------------------------------------------------
    # MQTT -> Caseta (commands)
    # ------------------------------------------------------------------

    def _on_mqtt_connect(self, client, userdata, flags, reason_code, properties):
        """MQTT connected — subscribe to command topics."""
        if reason_code == 0:
            prefix = self._cfg["topic_prefix"]
            topic = f"{prefix}/+/set"
            client.subscribe(topic, qos=1)
            log.info("MQTT connected, subscribed to %s", topic)
        else:
            log.error("MQTT connect failed: %s", reason_code)

    def _on_mqtt_message(self, client, userdata, msg):
        """MQTT message received — dispatch command to Caseta bridge.

        Runs in paho's background thread, so we schedule the async
        command handler onto the main asyncio event loop.
        """
        if self._loop is None:
            return
        asyncio.run_coroutine_threadsafe(
            self._handle_command(msg),
            self._loop,
        )

    async def _handle_command(self, msg) -> None:
        """Parse and execute an MQTT command message."""
        prefix = self._cfg["topic_prefix"]

        # Extract device name from topic: caseta/<name>/set
        parts = msg.topic.split("/")
        if len(parts) != 3 or parts[0] != prefix or parts[2] != "set":
            log.warning("ignoring unexpected topic: %s", msg.topic)
            return

        device_name = parts[1]
        device_id = self._name_to_id.get(device_name)
        if device_id is None:
            log.warning("unknown device name in command: %s", device_name)
            return

        try:
            cmd = json.loads(msg.payload)
        except json.JSONDecodeError:
            log.warning("invalid JSON in command: %s", msg.payload)
            return

        action = cmd.get("action")
        log.info("command: %s -> %s (device_id=%s)", action, device_name, device_id)

        try:
            if action == "turn_on":
                await self._bridge.turn_on(device_id)
            elif action == "turn_off":
                await self._bridge.turn_off(device_id)
            elif action == "set_level":
                level = cmd.get("level")
                if level is None:
                    log.warning("set_level command missing 'level' field")
                    return
                fade_time = cmd.get("fade_time")
                if fade_time is not None:
                    await self._bridge.set_value(device_id, level, fade_time)
                else:
                    await self._bridge.set_value(device_id, level)
            else:
                log.warning("unknown action: %s", action)
        except Exception:
            log.exception("failed to execute command: %s on %s", action, device_name)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    config_path = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_CONFIG_PATH

    # Structured logging to stdout (journald picks it up)
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        datefmt="%Y-%m-%dT%H:%M:%S",
        stream=sys.stdout,
    )

    log.info("starting caseta-bridge, config=%s", config_path)

    bridge = CasetaBridge(config_path)
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)

    # Signal handlers
    def handle_shutdown(sig):
        log.info("received %s", sig.name)
        loop.create_task(bridge.stop())
        # Give stop() a moment then halt the loop
        loop.call_later(2, loop.stop)

    def handle_reload():
        log.info("received SIGHUP")
        loop.create_task(bridge.reload_config())

    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, handle_shutdown, sig)
    loop.add_signal_handler(signal.SIGHUP, handle_reload)

    try:
        loop.run_until_complete(bridge.start())
        loop.run_forever()
    except KeyboardInterrupt:
        pass
    finally:
        loop.run_until_complete(bridge.stop())
        loop.close()


if __name__ == "__main__":
    main()
