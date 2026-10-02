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
| `GET /v1/sources`, mute/policy, quiet hours | see **Notification sources** below |
| `GET /v1/rules` | all Marshal rules from the mounted `rules.yaml`, with `muted`/`muted_until` and `owner` (Marshal's retained `state_topic`: automation/override/parked). Automations live here; notifications live under `/v1/sources` |
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

## Notification sources (0.3.0 contract)

The things that notify you are *sources*: every Marshal rule with an `alert:`
block, and every Frigate camera. Action-only Marshal rules (the nightlights)
are not sources and never appear here; parking them is a separate concern
(see `/v1/rules/{name}/enable`) and the app does not offer it.

A source id is the alert's `rule` field: the Marshal rule name, or
`frigate_<camera>` for a camera. Mutes and policies are keyed by that id.

```json
GET /v1/sources
{
  "sources": [
    { "id": "large_garage_door_left_open", "kind": "sensor", "name": "Large garage door left open",
      "rule": "large_garage_door_left_open", "severity": "warning", "repeat_minutes": 60,
      "muted": false, "muted_until": null,
      "policy": { "repeats": "critical", "objects": [], "always_notify": false } },
    { "id": "frigate_driveway", "kind": "camera", "name": "Driveway", "camera": "driveway",
      "muted": true, "muted_until": "2026-10-02T07:00:00Z",
      "policy": { "repeats": "none", "objects": ["person"], "always_notify": false } }
  ],
  "quiet_hours": { "enabled": true, "start": "23:00", "end": "07:00", "timezone": "America/Chicago", "active_now": false }
}
```

| Route | Body | Returns |
|---|---|---|
| `GET /v1/sources` | | sources + quiet hours (above) |
| `POST /v1/sources/{id}/mute` | `{"minutes": n}` or `{"minutes": null}` (until unmuted) or `{"until": "<RFC3339>"}` | the source |
| `DELETE /v1/sources/{id}/mute` | | the source |
| `PUT /v1/sources/{id}/policy` | any subset of `{"repeats": "none"\|"critical"\|"all", "objects": [..], "always_notify": bool}` | the source |
| `GET /v1/settings/quiet-hours` | | `{enabled, start, end, timezone, active_now}` |
| `PUT /v1/settings/quiet-hours` | any subset of `{"enabled": bool, "start": "HH:MM", "end": "HH:MM"}` | same |

Policy fields:

- `repeats` (sensor sources): which Marshal `repeat` messages push. `none`,
  `critical` (default: only critical rules), or `all`. The first sighting
  always pushes unless muted or in quiet hours.
- `objects` (camera sources): push only when the review's objects include one
  of these labels (Frigate labels: `person`, `car`, `dog`, …). Empty means
  any object. Default empty.
- `always_notify`: exempt from quiet hours. Default false.

Push decision, in order, for every alert event:

1. Is it a pushable event? First sighting: yes. Sensor repeat: by `repeats`.
   Anything else (resolved, ended, Frigate merges): no.
2. Muted? No push. Expired mutes are ignored.
3. Camera `objects` filter fails? No push.
4. Quiet hours active and not `always_notify`? No push.

Nothing above changes what is *recorded*: every alert is stored and streamed
regardless; policy only decides the push.

`POST /v1/rules/{name}/mute` and `DELETE` remain as aliases for sources that
are Marshal rules; new clients should use `/v1/sources`.

## Dashboard alerts (0.4.0)

ts-store alert rules post to the dashboard, which stores them and shows them
in its bell with the dashboard they relate to. sentinel subscribes to the
dashboard's own alert feed (`GET /api/events/stream`, SSE) with a view-only
API key and mirrors each alert as `kind: dashboard`. No dashboard change is
needed; the dashboard stays the system of record.

- **Identity.** One sentinel alert per dashboard alert id. Source id for mute
  and policy is `dashboard_<slug>` of the rule name (lowercase, runs of
  non-alphanumerics to `-`), and such a source appears in `/v1/sources`
  once its rule has fired at least once.
- **Link.** When the alert carries a `dashboard_id`, the alert JSON has
  `link`: `<DASHBOARD_PUBLIC_URL>/view/dashboards/<id>?var_<name>=<value>…&user_id=<guid>`,
  the same URL the bell's "open dashboard" builds, plus the user id so a
  fresh Safari is signed in (prod uses legacy GUID auth). `dashboard`
  carries `alert_id`, `rule_name`, `store`, `subtitle`, `dashboard_id`,
  `dashboard_vars`.
- **Lifecycle.** Created `active` with `started_at = fired_at`. Resolved when
  dismissed in the dashboard (reconciled every 5 minutes against the unseen
  list) or resolved in the app, which also marks it seen in the dashboard
  when `DASHBOARD_MARK_SEEN=true` (seen is global there).
- **Gaps.** The SSE feed has no replay, so sentinel backfills from
  `GET /api/alerts` on start and after every reconnect; an alert that fired
  while sentinel was down still arrives once. Severity maps
  `error→critical`, `warning→warning`, `info→info` (the dashboard sends
  `warning` for everything today).
- **Push.** Policy and quiet hours apply like any source (`repeats` is moot:
  every dashboard alert is a new id). The payload carries `link` and sets
  `aps.category = "dashboard"` so the app can offer an "Open Dashboard"
  notification action.
- **Health.** `/healthz` reports `dashboard: {enabled, connected, last_event, last_error}`.

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
| `DASHBOARD_URL`, `DASHBOARD_API_KEY` | unset | both required for the dashboard feed; otherwise it is off |
| `DASHBOARD_PUBLIC_URL` | unset | base of the deep links the phone opens |
| `DASHBOARD_LINK_USER_ID` | unset | appended to links as `user_id` |
| `DASHBOARD_MARK_SEEN` | `true` | app resolve → dashboard seen |

## Deploy

CI builds `ghcr.io/trv-enterprises/sentinel:<version>` from a tag:

```bash
git tag -a sentinel/v0.1.0 -m "sentinel v0.1.0" && git push origin sentinel/v0.1.0
```

Then from `homelab-deploy`: `make deploy-sentinel SENTINEL_VERSION=0.1.0`.
