#!/usr/bin/env python3
"""Stream a browser dashboard as change-driven MJPEG, driven over MQTT.

A Chromium kiosk renders the dashboard on a virtual X display. When someone
is watching -- an HTTP client on /stream.mjpeg, or an MQTT heartbeat on
<topic_prefix>/alive -- ffmpeg captures the display, drops frames that did
not change, and JPEG-encodes the rest; this process fans the frames out to
every connected client. Nobody watching for a grace period: capture stops.
The browser stays warm so the next viewer never waits on a page load.

The stream is continuous. Navigation, clicks and keys arrive on
<topic_prefix>/cmd and are applied to the same browser, so the picture just
changes -- the encoder is never restarted by a command. A "dashboard" cmd
action is forwarded instead to the dashboard app's own MQTT command bus (see
mqtt_bridge.MqttBridge.publish_dashboard_command), so the TV can drive
dashboard-level behaviour without browser input.

Clickable regions (links, buttons, tabs, inputs) are published on
<topic_prefix>/regions as normalised rectangles after each navigation and
whenever the page settles, so a TV client can move focus between them with
a remote and click by id; a free pointer can click by normalised x,y.

Configuration is by a YAML file; run with --config /path/to/config.yaml
(see config.yaml.example for the shape and systemd/stream-display.service
for the invocation). Every setting has a default so the file may be
partial. DISPLAY (the X display to capture) remains an environment
variable -- it names the Xvfb display stream-xvfb.service starts, not a
setting the Ansible role templates into the config file.
"""

import argparse
import asyncio
import collections
import concurrent.futures
import dataclasses
import logging
import os
import signal
import time
from typing import Any, Optional

import yaml
from aiohttp import web
from playwright.async_api import async_playwright
import numpy as np
from turbojpeg import TurboJPEG, TJSAMP_420

from mqtt_bridge import MqttBridge

log = logging.getLogger("stream-display")

# ---- configuration ---------------------------------------------------------


@dataclasses.dataclass
class ScreenConfig:
    width: int = 3840
    height: int = 2160


@dataclasses.dataclass
class DisplayConfig:
    name: str = "tv"
    start_url: str = ""
    screen: ScreenConfig = dataclasses.field(default_factory=ScreenConfig)
    device_scale: float = 2
    max_fps: int = 30
    jpeg_quality: int = 90          # ffmpeg -q:v, 2 (best) .. 31
    http_port: int = 8090
    idle_grace_s: float = 10       # no viewer -> stop capture
    alive_ttl_s: float = 15        # heartbeat validity
    keepalive_s: float = 2         # resend last frame when static
    vaapi_device: str = ""         # set to use hardware JPEG encode


@dataclasses.dataclass
class MqttConfig:
    broker: str = "127.0.0.1"
    port: int = 1883
    topic_prefix: str = ""         # default: "display/<display.name>"
    dashboard_command_topic: str = "dashboard/cmd"
    dashboard_client_id: str = ""  # reserved; appended as "/<id>" when set


@dataclasses.dataclass
class LoggingConfig:
    level: str = "INFO"


@dataclasses.dataclass
class Config:
    display: DisplayConfig = dataclasses.field(default_factory=DisplayConfig)
    mqtt: MqttConfig = dataclasses.field(default_factory=MqttConfig)
    logging: LoggingConfig = dataclasses.field(default_factory=LoggingConfig)


class ConfigError(Exception):
    """Raised for a malformed or incomplete config file."""


def _merge(defaults, overrides: dict):
    """Build a dataclass instance from `defaults`, overriding fields present in `overrides`."""
    if not overrides:
        return defaults
    kwargs = {}
    for f in dataclasses.fields(defaults):
        if f.name not in overrides:
            continue
        value = overrides[f.name]
        current = getattr(defaults, f.name)
        if dataclasses.is_dataclass(current) and isinstance(value, dict):
            kwargs[f.name] = _merge(current, value)
        else:
            kwargs[f.name] = value
    return dataclasses.replace(defaults, **kwargs)


def load_config(path: Optional[str]) -> Config:
    """Load config from a YAML file at `path`, filling in defaults for anything missing.

    Fails fast with ConfigError if the file exists but doesn't parse as a
    mapping, or if display.start_url ends up empty.
    """
    raw: dict[str, Any] = {}
    if path and os.path.exists(path):
        with open(path, "r") as f:
            loaded = yaml.safe_load(f) or {}
        if not isinstance(loaded, dict):
            raise ConfigError(f"{path}: expected a YAML mapping at the top level")
        raw = loaded
    elif path:
        raise ConfigError(f"config file not found: {path}")

    cfg = _merge(Config(), raw)

    if not cfg.display.start_url:
        raise ConfigError(
            "display.start_url is required -- set it in the config file "
            f"({path or 'no --config given'})"
        )

    if not cfg.mqtt.topic_prefix:
        cfg.mqtt.topic_prefix = f"display/{cfg.display.name}"

    return cfg


REGIONS_JS = """
() => {
  const sel = 'a[href], button, [role=button], [role=tab], [role=link], [role=menuitem], input, select, textarea, summary, [onclick]';
  const out = []; let i = 0;
  for (const el of document.querySelectorAll(sel)) {
    const r = el.getBoundingClientRect();
    if (r.width < 8 || r.height < 8) continue;
    if (r.bottom < 0 || r.right < 0 || r.top > innerHeight || r.left > innerWidth) continue;
    const st = getComputedStyle(el);
    if (st.visibility === 'hidden' || st.display === 'none' || st.pointerEvents === 'none') continue;
    const label = (el.getAttribute('aria-label') || el.innerText || el.value || el.title || el.alt || '').trim().slice(0, 60);
    out.push({ id: 'r' + (i++), label, x: r.left / innerWidth, y: r.top / innerHeight,
               w: r.width / innerWidth, h: r.height / innerHeight });
  }
  return out;
}
"""


class Capture:
    """x11grab raw frames -> subsampled change check -> per-frame parallel JPEG.

    MJPEG frames are independent, so they are encoded in a thread pool one
    frame per task (libjpeg-turbo releases the GIL). That gives the pool's
    worth of throughput with no pipeline delay: a frame costs exactly one
    encode, unlike ffmpeg's frame-threaded MJPEG, which buffers several frames
    before emitting any and so never flushes on a mostly static screen.
    Change detection compares every 4th row and column of the luma plane
    against the last frame sent -- far cheaper than mpdecimate at 4K.
    """

    def __init__(self, cfg, x_display=":99"):
        self.cfg = cfg
        self.x_display = x_display
        self.proc = None
        self.subscribers: set[asyncio.Queue] = set()
        self.last_frame: bytes | None = None
        self.last_sent = 0.0
        self.frames = 0          # frames sent
        self.grabbed = 0         # frames read from ffmpeg
        self._pool = concurrent.futures.ThreadPoolExecutor(max_workers=max(2, (os.cpu_count() or 4)))
        self._jpeg = TurboJPEG()
        self._prev = None

    def running(self):
        return self.proc is not None and self.proc.returncode is None

    async def start(self):
        if self.running():
            return
        w, h = self.cfg.screen.width, self.cfg.screen.height
        cmd = ["ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin",
               "-f", "x11grab", "-framerate", str(self.cfg.max_fps), "-video_size", f"{w}x{h}",
               "-draw_mouse", "0", "-i", f"{self.x_display}.0",
               "-pix_fmt", "yuv420p", "-f", "rawvideo", "pipe:1"]
        self.proc = await asyncio.create_subprocess_exec(*cmd, stdout=asyncio.subprocess.PIPE,
                                                         limit=w * h * 3)
        self.frames = self.grabbed = 0
        self._prev = None
        asyncio.create_task(self._pump())
        log.info("capture started (%dx%d, max %d fps, turbojpeg q=%d, %d encoder threads)",
                 w, h, self.cfg.max_fps, self.cfg.jpeg_quality, self._pool._max_workers)

    async def stop(self):
        if not self.running():
            return
        self.proc.terminate()
        try:
            await asyncio.wait_for(self.proc.wait(), 5)
        except asyncio.TimeoutError:
            self.proc.kill()
        log.info("capture stopped: %d frames sent of %d grabbed", self.frames, self.grabbed)

    def _changed(self, y: memoryview) -> bool:
        """True if the subsampled luma differs from the last frame we sent."""
        w, h = self.cfg.screen.width, self.cfg.screen.height
        cur = np.frombuffer(y, dtype=np.uint8).reshape(h, w)[::4, ::4]
        if self._prev is None:
            self._prev = cur.copy(); return True
        # Sensitive on purpose: live camera tiles change by small amounts and
        # must stream; a genuinely static screen still produces nothing. At
        # ~130 KB per 4K frame, even 30 fps of camera motion is only ~30 Mb/s.
        diff = np.count_nonzero(np.abs(cur.astype(np.int16) - self._prev) > 6)
        if diff > cur.size * 0.0001:          # >0.01 % of samples moved
            self._prev = cur.copy(); return True
        return False

    async def _pump(self):
        proc = self.proc
        w, h = self.cfg.screen.width, self.cfg.screen.height
        n = w * h * 3 // 2
        loop = asyncio.get_running_loop()
        pending: collections.deque = collections.deque()   # keep frames in order
        try:
            while True:
                try:
                    raw = await proc.stdout.readexactly(n)
                except asyncio.IncompleteReadError:
                    break
                self.grabbed += 1
                if self._changed(memoryview(raw)[: w * h]):
                    pending.append(loop.run_in_executor(self._pool, self._jpeg.encode_from_yuv,
                                                        raw, h, w, self.cfg.jpeg_quality, TJSAMP_420))
                # Publish finished encodes in order on EVERY grabbed frame -- not only
                # when a changed one arrives, or a lone frame would sit here until the
                # next change. Bound in-flight work to the pool size.
                while pending and (pending[0].done() or len(pending) >= self._pool._max_workers):
                    self._publish(await pending.popleft())
            while pending:
                self._publish(await pending.popleft())
        except Exception as e:
            log.warning("capture pump ended: %s", e)
        finally:
            if proc is self.proc:
                self.proc = None

    def _publish(self, frame: bytes):
        self.last_frame, self.last_sent, self.frames = frame, time.monotonic(), self.frames + 1
        for q in list(self.subscribers):
            if q.full():                      # slow client: drop its oldest frame
                try: q.get_nowait()
                except asyncio.QueueEmpty: pass
            q.put_nowait(frame)

    def subscribe(self) -> asyncio.Queue:
        q = asyncio.Queue(maxsize=2)
        self.subscribers.add(q)
        return q

    def unsubscribe(self, q):
        self.subscribers.discard(q)


class Display:
    def __init__(self, cfg: Config, x_display: str):
        self.cfg = cfg
        self.x_display = x_display
        self.capture = Capture(cfg.display, x_display)
        self.page = None
        self.regions = []
        self.cmd_queue: asyncio.Queue = asyncio.Queue()

        # CSS-pixel viewport the page lays out in; normalised coordinates map to it.
        self.view_w = int(cfg.display.screen.width / cfg.display.device_scale)
        self.view_h = int(cfg.display.screen.height / cfg.display.device_scale)

        self.mqtt = MqttBridge(
            broker=cfg.mqtt.broker,
            port=cfg.mqtt.port,
            topic_prefix=cfg.mqtt.topic_prefix,
            dashboard_command_topic=cfg.mqtt.dashboard_command_topic,
            dashboard_client_id=cfg.mqtt.dashboard_client_id,
            client_id=f"stream-display-{cfg.display.name}",
        )

    # ---- browser -----------------------------------------------------------
    async def start_browser(self):
        d = self.cfg.display
        w, h = d.screen.width, d.screen.height
        pw = await async_playwright().start()
        # A persistent context drives the browser's own first window, which is
        # the one --kiosk applies to; launch()+new_context() opens a second,
        # chromed window and the kiosk flag is ignored. The profile directory
        # also keeps the dashboard's identity (localStorage) across restarts.
        ctx = await pw.chromium.launch_persistent_context(
            os.path.join(os.path.dirname(os.path.abspath(__file__)), "profile"),
            headless=False, no_viewport=True, ignore_https_errors=True,
            env={**os.environ, "DISPLAY": os.environ.get("DISPLAY", ":99")},
            args=["--kiosk", "--start-fullscreen", "--no-first-run", "--disable-infobars",
                  # --window-size is in device-independent pixels: at scale 2 the
                  # physical 3840x2160 screen is a 1920x1080 window. Asking for the
                  # physical size here would open a window four times the screen.
                  "--disable-session-crashed-bubble", "--window-position=0,0",
                  f"--window-size={int(w / d.device_scale)},{int(h / d.device_scale)}",
                  f"--force-device-scale-factor={d.device_scale}",
                  "--autoplay-policy=no-user-gesture-required", "--noerrdialogs",
                  "--disable-translate", "--hide-scrollbars"],
            ignore_default_args=["--enable-automation"])
        self.page = ctx.pages[0] if ctx.pages else await ctx.new_page()
        self.page.on("load", lambda _: asyncio.create_task(self.publish_regions("load")))
        await self.page.goto(d.start_url, wait_until="domcontentloaded")
        vp = self.page.viewport_size or {"width": self.view_w, "height": self.view_h}
        log.info("browser at %s (%dx%d css px, scale %s)", d.start_url, vp["width"], vp["height"], d.device_scale)

    async def publish_regions(self, reason=""):
        try:
            await self.page.wait_for_load_state("networkidle", timeout=5000)
        except Exception:
            pass
        try:
            self.regions = await self.page.evaluate(REGIONS_JS)
        except Exception as e:
            log.warning("regions failed: %s", e); return
        self.mqtt.publish_regions({"url": self.page.url, "count": len(self.regions), "regions": self.regions})
        log.info("regions: %d (%s)", len(self.regions), reason)

    async def handle_cmd(self, c: dict):
        a = c.get("action"); p = self.page
        try:
            if a == "navigate":
                await p.goto(c["url"], wait_until="domcontentloaded")
            elif a == "click":
                if "id" in c:
                    r = next((r for r in self.regions if r["id"] == c["id"]), None)
                    if not r: log.warning("unknown region %s", c["id"]); return
                    x, y = (r["x"] + r["w"] / 2) * self.view_w, (r["y"] + r["h"] / 2) * self.view_h
                else:
                    x, y = float(c["x"]) * self.view_w, float(c["y"]) * self.view_h
                await p.mouse.click(x, y)
                asyncio.create_task(self.publish_regions("click"))
            elif a == "move":
                await p.mouse.move(float(c["x"]) * self.view_w, float(c["y"]) * self.view_h)
            elif a == "scroll":
                if "x" in c: await p.mouse.move(float(c["x"]) * self.view_w, float(c["y"]) * self.view_h)
                await p.mouse.wheel(float(c.get("dx", 0)), float(c.get("dy", 0)))
                asyncio.create_task(self.publish_regions("scroll"))
            elif a == "key":
                await p.keyboard.press(c["key"])
            elif a == "type":
                await p.keyboard.type(c["text"])
            elif a == "reload":
                await p.reload(wait_until="domcontentloaded")
            elif a == "back":
                await p.go_back()
            elif a == "forward":
                await p.go_forward()
            elif a == "regions":
                await self.publish_regions("requested")
            else:
                log.warning("unknown action %r", a)
        except Exception as e:
            log.warning("command %s failed: %s", a, e)
        self.publish_state()

    # ---- lifecycle -----------------------------------------------------------
    def wanted(self):
        d = self.cfg.display
        return bool(self.capture.subscribers) or (time.monotonic() - self.mqtt.last_alive) < d.alive_ttl_s

    async def lifecycle(self):
        idle_since = None
        idle_grace = self.cfg.display.idle_grace_s
        while True:
            if self.wanted():
                idle_since = None
                if not self.capture.running():
                    await self.capture.start(); self.publish_state()
            elif self.capture.running():
                idle_since = idle_since or time.monotonic()
                if time.monotonic() - idle_since > idle_grace:
                    await self.capture.stop(); self.publish_state()
            await asyncio.sleep(1)

    async def commands(self):
        while True:
            await self.handle_cmd(await self.cmd_queue.get())

    def publish_state(self):
        d = self.cfg.display
        st = {"online": True, "url": self.page.url if self.page else None,
              "capturing": self.capture.running(), "viewers": len(self.capture.subscribers),
              "alive": (time.monotonic() - self.mqtt.last_alive) < d.alive_ttl_s,
              "alive_info": self.mqtt.last_alive_payload,
              "screen": [d.screen.width, d.screen.height], "viewport": [self.view_w, self.view_h],
              "frames": self.capture.frames}
        self.mqtt.publish_state(st)

    # ---- http ----------------------------------------------------------------
    async def stream(self, request):
        resp = web.StreamResponse(headers={"Content-Type": "multipart/x-mixed-replace; boundary=frame",
                                           "Cache-Control": "no-cache", "Connection": "close"})
        await resp.prepare(request)
        q = self.capture.subscribe(); self.publish_state()
        log.info("viewer connected from %s (%d)", request.remote, len(self.capture.subscribers))
        keepalive_s = self.cfg.display.keepalive_s
        try:
            if self.capture.last_frame:            # show something immediately
                await self._write(resp, self.capture.last_frame)
            while True:
                try:
                    frame = await asyncio.wait_for(q.get(), keepalive_s)
                except asyncio.TimeoutError:       # static screen: repeat so clients don't time out
                    frame = self.capture.last_frame
                    if not frame: continue
                await self._write(resp, frame)
        except (ConnectionResetError, asyncio.CancelledError, ConnectionError):
            pass
        finally:
            self.capture.unsubscribe(q); self.publish_state()
            log.info("viewer left (%d)", len(self.capture.subscribers))
        return resp

    @staticmethod
    async def _write(resp, frame):
        await resp.write(b"--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n" % len(frame)
                         + frame + b"\r\n")

    async def snapshot(self, request):
        f = self.capture.last_frame
        if not f:
            if not self.capture.running():
                await self.capture.start()
            for _ in range(50):
                await asyncio.sleep(0.1)
                if self.capture.last_frame: f = self.capture.last_frame; break
        return web.Response(body=f or b"", content_type="image/jpeg") if f else web.Response(status=503)

    async def health(self, request):
        return web.json_response({"name": self.cfg.display.name, "url": self.page.url if self.page else None,
                                  "capturing": self.capture.running(), "viewers": len(self.capture.subscribers),
                                  "frames": self.capture.frames, "regions": len(self.regions)})

    async def regions_json(self, request):
        return web.json_response({"url": self.page.url, "regions": self.regions})

    async def cmd_http(self, request):
        """Same commands over HTTP, for testing without an MQTT client."""
        await self.cmd_queue.put(await request.json())
        return web.json_response({"queued": True})

    async def run(self):
        await self.start_browser()
        self.mqtt.start(self.cmd_queue)
        app = web.Application()
        app.add_routes([web.get("/stream.mjpeg", self.stream), web.get("/snapshot.jpg", self.snapshot),
                        web.get("/health", self.health), web.get("/regions.json", self.regions_json),
                        web.post("/cmd", self.cmd_http)])
        runner = web.AppRunner(app); await runner.setup()
        await web.TCPSite(runner, "0.0.0.0", self.cfg.display.http_port).start()
        log.info("http on :%d  (/stream.mjpeg /snapshot.jpg /health /regions.json POST /cmd)",
                 self.cfg.display.http_port)
        await self.publish_regions("start"); self.publish_state()
        await asyncio.gather(self.lifecycle(), self.commands())


def parse_args():
    p = argparse.ArgumentParser(description="Stream a browser dashboard as change-driven MJPEG, MQTT-controlled.")
    p.add_argument("--config", default=None, help="Path to config.yaml (see config.yaml.example)")
    return p.parse_args()


def main():
    args = parse_args()
    try:
        cfg = load_config(args.config)
    except ConfigError as e:
        # Logging isn't configured yet -- this is a startup-time failure the
        # operator needs to see regardless of the configured log level.
        print(f"stream-display: config error: {e}")
        raise SystemExit(1)

    logging.basicConfig(level=cfg.logging.level, format="%(asctime)s %(levelname)s %(message)s")

    x_display = os.environ.get("DISPLAY", ":99")

    d = Display(cfg, x_display)
    loop = asyncio.new_event_loop()
    for s in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(s, lambda: [t.cancel() for t in asyncio.all_tasks(loop)])
    try:
        loop.run_until_complete(d.run())
    except asyncio.CancelledError:
        pass
    finally:
        loop.run_until_complete(d.capture.stop())


if __name__ == "__main__":
    main()
