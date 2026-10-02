# sentinel

Homelab alerts on an iPhone. Sentinel subscribes to the two alert topics on
the services broker, keeps the history in SQLite, pushes new alerts through
APNs, and serves a small API for the [trv-sentinel](https://github.com/trv-enterprises/trv-sentinel)
iOS app, including a media proxy so the app can play Frigate clips.

```
Marshal ──sensors/alerts──┐                              ┌─ APNs ──► iPhone
                          ├─► Mosquitto ─► sentinel ─────┤
Frigate ─frigate/reviews─┘                               ├─ HTTPS API (tailscale serve) ◄── app
                                                         └─ clips/snapshots proxied from Frigate
```

## What it ingests

| Topic | Producer | Becomes |
|---|---|---|
| `sensors/alerts` | Marshal | `kind: sensor` alert keyed by rule name while active |
| `frigate/reviews` | Frigate (via the NVR bridge) | `kind: camera` alert keyed by review id, created the first time a review carries `severity: alert` |

A sensor alert is created on `new` (or on an orphan `repeat`), updated on
`repeat`, closed on `resolved`. A camera alert merges detections on
`update` and closes on `end`. Alerts with no traffic past their rule's
repeat window go `stale` (Marshal forgets state on restart and never sends
`resolved` for those).

## API

All routes need `Authorization: Bearer <API_TOKEN>` except `/healthz`.

| Route | Purpose |
|---|---|
| `GET /v1/alerts?status=&kind=&since=&limit=` | newest-updated first |
| `GET /v1/alerts/{id}` | one alert, with signed media links for camera alerts |
| `POST /v1/alerts/{id}/resolve` | human resolve |
| `GET /v1/events` | SSE stream of `created` / `updated` events |
| `GET /v1/media/...` | Frigate proxy; accepts a signed URL instead of the bearer |
| `POST /v1/devices` `{token, env, name, app_version}` | register an APNs device token (upsert) |
| `GET /v1/devices`, `DELETE /v1/devices/{token}` | |
| `POST /v1/push/test` `{token?, env?}` | send a test push |
| `GET /v1/rules` | Marshal rules from the mounted `rules.yaml`, plus `muted`/`muted_until` (sentinel) and `owner` (Marshal's retained `state_topic`: automation/override/parked) |
| `POST /v1/rules/{name}/mute` `{minutes: n\|null}` | suppress pushes for a rule (null = until unmuted); the rule still fires and is listed |
| `DELETE /v1/rules/{name}/mute` | lift the mute |
| `POST /v1/rules/{name}/enable` `{enabled: bool}` | publish to the rule's Marshal `enable_topic` (parks/resumes every rule sharing that topic); 409 for alert-only rules, which Marshal cannot park |
| `GET /healthz` | MQTT liveness; 503 when the heartbeat loopback is stale |

Media links are signed with `hmac-sha256(API_TOKEN, path|exp)` and expire
after 12 hours, so a URL handed to AVPlayer or the notification extension
never carries the token itself. A camera alert's `media` object carries:

| key | what | use |
|---|---|---|
| `thumb` | review thumbnail (webp) | list rows, push attachment |
| `snapshot` | first event's snapshot (jpg); 404 when Frigate kept none | detail header |
| `hls` | HLS playlist for the first event | **AVPlayer**: Frigate's mp4 endpoint ignores `Range`, HLS seeks properly |
| `hls_range` | HLS playlist for the whole review span | same, once `end_time` is known |
| `clip`, `recording` | the mp4 equivalents | download / share |

Playlists are rewritten on the way through so every segment line carries its
own signed query (AVPlayer drops the playlist's query when resolving relative
segment names).

## Configuration (env)

| Var | Default | |
|---|---|---|
| `MQTT_BROKER` | `tcp://mosquitto:1883` | |
| `MQTT_CLIENT_ID` | `sentinel` | fixed on purpose: persistent session |
| `HTTP_ADDR` | `:8070` | |
| `PUBLIC_BASE_URL` | `http://localhost:8070` | what the app reaches; used in signed links |
| `API_TOKEN` | required | ≥16 chars |
| `DB_PATH` | `/data/sentinel.db` | |
| `RETENTION` | `720h` | |
| `FRIGATE_URL` | `http://192.168.1.156:5000` | |
| `RULES_PATH` | `/etc/marshal/rules.yaml` | |
| `APNS_KEY_B64`, `APNS_KEY_ID`, `APNS_TEAM_ID`, `APNS_BUNDLE_ID` | unset | all four required for push; otherwise the server runs with push disabled |

## Deploy

CI builds `ghcr.io/trv-enterprises/sentinel:<version>` from a tag:

```bash
git tag -a sentinel/v0.1.0 -m "sentinel v0.1.0" && git push origin sentinel/v0.1.0
```

Then from `homelab-deploy`: `make deploy-sentinel SENTINEL_VERSION=0.1.0`.
