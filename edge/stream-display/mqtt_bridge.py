"""MQTT bridge for stream-display.

Wraps paho-mqtt in the same style as the kiosk voice pipeline's
DisplayCommander (trv-kiosk/voice/commander.py::_start_mqtt): connect_async +
loop_start so paho owns retry (including the very first connect), a
last-will "state" message, and small logged publish helpers.

The service loop is asyncio; paho's callbacks run on its own network thread.
Inbound commands are handed to the service through call_soon_threadsafe into
an asyncio.Queue, exactly as stream_display.py already did before this
module existed.
"""

import asyncio
import json
import logging
import time
from typing import Optional

import paho.mqtt.client as mqtt

log = logging.getLogger("stream-display.mqtt")


class MqttBridge:
    """MQTT connectivity for one display: cmd/alive in, state/regions out."""

    def __init__(
        self,
        broker: str,
        port: int,
        topic_prefix: str,
        dashboard_command_topic: str = "dashboard/cmd",
        dashboard_client_id: str = "",
        client_id: str = "",
    ):
        self.broker = broker
        self.port = port
        self.topic_prefix = topic_prefix.rstrip("/")
        self.dashboard_command_topic = dashboard_command_topic
        self.dashboard_client_id = dashboard_client_id

        self.t_cmd = f"{self.topic_prefix}/cmd"
        self.t_alive = f"{self.topic_prefix}/alive"
        self.t_state = f"{self.topic_prefix}/state"
        self.t_regions = f"{self.topic_prefix}/regions"

        self.last_alive = 0.0
        self.last_alive_payload: dict = {}

        self._loop: Optional[asyncio.AbstractEventLoop] = None
        self._cmd_queue: Optional[asyncio.Queue] = None

        self.client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=client_id or None)
        self.client.on_connect = self._on_connect
        self.client.on_disconnect = self._on_disconnect
        self.client.on_message = self._on_message
        self.client.reconnect_delay_set(min_delay=1, max_delay=60)
        self.client.will_set(self.t_state, json.dumps({"online": False}), retain=True)

    def start(self, cmd_queue: asyncio.Queue):
        """Start the client. Must be called from within the running asyncio loop."""
        self._loop = asyncio.get_running_loop()
        self._cmd_queue = cmd_queue

        # connect_async + loop_start lets paho own the connect retry. The
        # background thread retries on its own — including the very first
        # connect — so a boot-time "Network is unreachable" no longer wedges
        # the client in a permanently-disconnected state.
        self.client.connect_async(self.broker, self.port, 60)
        self.client.loop_start()
        log.info("mqtt client connecting to %s:%s", self.broker, self.port)

    def stop(self):
        self.client.loop_stop()
        self.client.disconnect()
        log.info("mqtt client stopped")

    # ---- callbacks ---------------------------------------------------------

    def _on_connect(self, client, userdata, flags, reason_code, properties):
        log.info("mqtt connected, subscribing to %s, %s", self.t_cmd, self.t_alive)
        client.subscribe([(self.t_cmd, 0), (self.t_alive, 0)])

    def _on_disconnect(self, client, userdata, flags, reason_code, properties):
        """Log disconnects so a silently-dropped broker link is visible."""
        log.warning("mqtt disconnected (reason_code=%s); paho will retry", reason_code)

    def _on_message(self, client, userdata, msg):
        if msg.topic == self.t_alive:
            self.last_alive = time.monotonic()
            if msg.payload:
                try:
                    self.last_alive_payload = json.loads(msg.payload.decode())
                except (ValueError, UnicodeDecodeError):
                    log.warning("bad alive payload")
            else:
                self.last_alive_payload = {}
            return

        try:
            command = json.loads(msg.payload.decode() or "{}")
        except (ValueError, UnicodeDecodeError):
            log.warning("bad command payload")
            return

        if command.get("action") == "dashboard":
            target = command.get("target")
            action = command.get("command")
            if target and action:
                self.publish_dashboard_command(target, action)
            else:
                log.warning("dashboard command missing target/command: %r", command)
            return

        if self._loop is not None and self._cmd_queue is not None:
            self._loop.call_soon_threadsafe(self._cmd_queue.put_nowait, command)

    # ---- publish helpers ----------------------------------------------------

    def publish_state(self, state: dict):
        self.client.publish(self.t_state, json.dumps(state), retain=True)

    def publish_regions(self, regions: dict):
        self.client.publish(self.t_regions, json.dumps(regions), retain=True)

    def publish_dashboard_command(self, target: str, action: str):
        """Publish to the dashboard app's global command bus.

        The dashboard subscribes to dashboard_command_topic and every viewer
        acts on commands whose `target` matches a component it renders.
        Payload: {"target": "<target>", "action": "<action>"}
        """
        if not action:
            log.warning("publish_dashboard_command called with empty action")
            return
        topic = self.dashboard_command_topic
        if self.dashboard_client_id:
            topic = f"{topic}/{self.dashboard_client_id}"
        payload = json.dumps({"target": target, "action": action})
        try:
            self.client.publish(topic, payload)
            log.info("published dashboard command to %s: %s", topic, payload)
        except Exception as e:
            log.error("failed to publish dashboard command: %s", e)
