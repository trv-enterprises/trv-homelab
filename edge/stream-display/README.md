# Stream Display

Runs a Chromium kiosk on a virtual X display (Xvfb) and streams it as
change-driven MJPEG over HTTP. Navigation, clicks, and keys are applied to
the same long-lived browser over MQTT, so the picture just changes — the
encoder is never restarted by a command. Clickable regions are published
after each navigation so a TV client can move focus with a remote and click
by id, or a free pointer can click by normalised x,y.

Runs natively (no Docker) in a privileged LXC. Chromium comes from
Playwright (`playwright install --with-deps chromium`), not apt/snap.

Configured from `/etc/stream-display/config.yaml`, rendered by the Ansible
role from a fixed template — see `config.yaml.j2` in
`tools/ansible/roles/stream-display/templates/` for the full shape.

## MQTT contract

Topics are namespaced under `display.name` from the config
(`topic_prefix: display/<name>`).

| Topic | Direction | Purpose |
|-------|-----------|---------|
| `display/<name>/cmd` | in | Commands (see below) |
| `display/<name>/alive` | in | Heartbeat — any payload keeps capture running |
| `display/<name>/state` | out, retained | `{online, url, capturing, viewers, alive, screen, viewport, frames}` |
| `display/<name>/regions` | out, retained | `{url, count, regions: [{id, label, x, y, w, h}]}`, normalised 0–1 |

### `cmd` actions (JSON payload, `{"action": ...}`)

| Action | Payload | Effect |
|--------|---------|--------|
| `navigate` | `{url}` | Load a new URL |
| `click` | `{id}` or `{x, y}` (normalised 0–1) | Click a published region by id, or a point |
| `move` | `{x, y}` (normalised 0–1) | Move the pointer without clicking |
| `scroll` | `{dx, dy[, x, y]}` | Wheel-scroll, optionally after moving to `x,y` first |
| `key` | `{key}` | Press a single key |
| `type` | `{text}` | Type a string |
| `reload` | — | Reload the current page |
| `back` | — | Browser back |
| `forward` | — | Browser forward |
| `regions` | — | Force a regions re-publish |
| `dashboard` | `{target, action}` | Forwarded verbatim to the dashboard's own command topic (`mqtt.dashboard_command_topic`), not applied locally |

## HTTP endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /stream.mjpeg` | Multipart MJPEG stream (`multipart/x-mixed-replace`) |
| `GET /snapshot.jpg` | Single current frame |
| `GET /health` | `{name, url, capturing, viewers, frames, regions}` |
| `GET /regions.json` | `{url, regions}` |
| `POST /cmd` | Same commands as MQTT `cmd`, for testing without an MQTT client |

## Capture lifecycle

`ffmpeg` only captures the X display while someone is watching: an HTTP
client connected to `/stream.mjpeg`, or an `alive` heartbeat received within
`alive_ttl_s`. With nobody watching, capture stops after `idle_grace_s`. The
browser itself stays warm the whole time — only the encoder starts and
stops — so the next viewer never waits on a page load. Frames that didn't
meaningfully change are dropped (`mpdecimate`) before JPEG encoding; set
`vaapi_device` in the config to use hardware JPEG encode instead of CPU.

## Deploy

```bash
make deploy-stream-display   # from homelab-deploy
```

Applies `tools/ansible/roles/stream-display/` (this repo) using real values
from `homelab-deploy/inventory/host_vars/stream.yml`.
