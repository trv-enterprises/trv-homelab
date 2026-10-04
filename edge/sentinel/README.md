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

All routes need `Authorization: Bearer <token>` except `/healthz`. The token
says who is asking: see **People** below.

| Route | Purpose |
|---|---|
| `GET /v1/alerts?status=&kind=&source=&from=&to=&since=&cursor=&limit=&scope=` | newest-updated first; see **History** below |
| `GET /v1/me`, `PUT /v1/me` `{outpost_user_id}` | who the token belongs to, and their Outpost user; see **People** |
| `GET /v1/alerts/{id}` | one alert, with signed media links for camera alerts |
| `POST /v1/alerts/{id}/resolve` | human resolve |
| `GET /v1/events` | SSE stream of `created` / `updated` events, the asker's own history only |
| `GET /v1/media/...` | Frigate proxy; accepts a signed URL instead of the bearer |
| `POST /v1/devices` `{token, env, name, app_version}` | register an APNs device token (upsert); the phone belongs to whoever registers it |
| `GET /v1/devices`, `DELETE /v1/devices/{token}` | |
| `POST /v1/push/test` `{token?, env?}` | send a test push (to the asker's phones when no token is given) |
| `GET /v1/sources`, mute/policy, quiet hours | see **Notification sources** below |
| `PUT /v1/sources/{id}/action` `{active: bool}` | switch a source's Marshal action on or off; see **Rule actions** below |
| `GET /v1/rules` | all Marshal rules from the mounted `rules.yaml`, with `muted`/`muted_until`, `owner` (Marshal's retained `state_topic`: automation/override/parked) and, for action rules, `active`. Automations live here; notifications live under `/v1/sources` |
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

## People (0.7.0)

Each person who runs the app has their own token, and the token is the only
thing that says who is asking. There are no accounts and no sign-in.

| What | Whose |
|---|---|
| Mutes (a source, or a whole kind), policy (`repeats`, `objects`, `always_notify`, `passive`), quiet hours | the person's own |
| Which phones get a push, and how | the person's own: a phone belongs to whoever registered it |
| History (see below), the Outpost user their dashboard links open as | the person's own |
| The alerts themselves, resolving one, the sources that exist | shared |
| A rule's action (`active_topic`) | shared: it is the house's, and lives on the broker |

Configuration: `API_TOKEN` is the **owner's** token and `API_PERSON` their id
(default `owner`). `API_PEOPLE` adds the others as comma-separated `id:token`.
Ids are lowercase (`[a-z0-9_-]`); tokens are at least 16 characters and all
different. The owner's token is also the key media links are signed with.

On its first start 0.7.0 gives the owner everything the single-person server
had: mutes, policies, quiet hours and every registered phone. That happens
once (`settings.people_adopted` records for whom) and the old tables are left
as they were, so rolling back to 0.6.0 finds its data unchanged. Anyone else
starts from defaults. Pick the owner's id before that first start: renaming
it later leaves the adopted settings under the old id.

`GET /v1/me` returns `{id, name, outpost_user_id}`. `PUT /v1/me`
`{"outpost_user_id": "<id>"}` sets the Outpost (dashboard) user that dashboard
links open as for this person, in API responses, on the SSE stream and in
their pushes. Empty clears it and the server's `DASHBOARD_LINK_USER_ID`
applies. The id signs a browser in: it is stored but never logged.

Every push decision below is made once per person, and only that person's
phones receive the result.

## History (0.7.0)

`GET /v1/alerts` lists newest-updated first.

| Parameter | |
|---|---|
| `kind` | `sensor`, `camera`, `dashboard`; several allowed (0.8.0) |
| `source` | a source id (the alert's `rule`): a Marshal rule, `frigate_<camera>`, `dashboard_<slug>`; several allowed (0.8.0) |
| `from`, `to` | RFC3339 bounds of a time range (0.8.0) |
| `status` | `active`, `resolved`, `ended`, `stale` |
| `since` | only alerts updated after this RFC3339 time |
| `limit` | page size, default 100, at most 500 |
| `cursor` | continue from a previous page's `next_cursor` |
| `scope` | `mine` (default) or `all` |

`kind` and `source` each take several values, repeated or comma-separated,
and together they are a **union**: alerts of any of those kinds, or from any
of those sources. `kind=camera&source=garage,hall` is every camera alert plus
those two sensors. (Before 0.8.0 each took one value and the two narrowed
each other.) At most 200 values in all.

An alert is inside `from`..`to` when it was open at any point in the range:
it started by `to`, and it was last updated at or after `from` or is still
active. A door opened at 23:50 and closed at 00:20 is in both days. Either
bound may be left out. The range narrows whatever `kind` and `source` select.

A full page carries `next_cursor`; pass it back as `cursor` for the next one.
It is opaque (the last alert's update time and id), so alerts updated in the
same instant are neither repeated nor skipped across a page boundary.

`scope=mine` is the asker's own history: everything except alerts that were
**hidden** from them when they fired. An alert is hidden from a person when
its first sighting was something they had asked not to hear about, a muted
source, a muted kind, or a camera review without one of their `objects`.
Being held by quiet hours or by the repeat policy does not hide it, so the
morning still shows what happened overnight. A mute set afterwards does not
reach back, and an alert that later does reach the person (the mute ran out
and a repeat pushed) becomes theirs again. `scope=all` is everything that was
recorded. `GET /v1/alerts/{id}` ignores all of this: a tapped notification
must never 404.

Alerts from before 0.7.0 are in everyone's history.

## Notification sources (0.3.0 contract)

The things that notify you are *sources*: every Marshal rule with an `alert:`
block, and every Frigate camera. Action-only Marshal rules are not sources
and never appear here, and neither does a rule whose alert is switched off in
the config (`alert.active: false`, Marshal 0.5.0), because it never fires.
Parking an automation is a separate concern (see `/v1/rules/{name}/enable`)
and the app does not offer it.

A source id is the alert's `rule` field: the Marshal rule name, or
`frigate_<camera>` for a camera. Mutes and policies are keyed by that id.

```json
GET /v1/sources
{
  "sources": [
    { "id": "large_garage_door_left_open", "kind": "sensor", "name": "Large garage door left open",
      "rule": "large_garage_door_left_open", "severity": "warning", "repeat_minutes": 60,
      "muted": false, "muted_until": null,
      "policy": { "repeats": "critical", "objects": [], "always_notify": false, "passive": false } },
    { "id": "frigate_driveway", "kind": "camera", "name": "Driveway", "camera": "driveway",
      "muted": true, "muted_until": "2026-10-02T07:00:00Z",
      "policy": { "repeats": "none", "objects": ["person"], "always_notify": false, "passive": false } }
  ],
  "quiet_hours": { "enabled": true, "start": "23:00", "end": "07:00", "timezone": "America/Chicago", "active_now": false }
}
```

| Route | Body | Returns |
|---|---|---|
| `GET /v1/sources` | | sources + quiet hours (above) |
| `POST /v1/sources/{id}/mute` | `{"minutes": n}` or `{"minutes": null}` (until unmuted) or `{"until": "<RFC3339>"}` | the source |
| `DELETE /v1/sources/{id}/mute` | | the source |
| `PUT /v1/sources/{id}/policy` | any subset of `{"repeats": "none"\|"critical"\|"all", "objects": [..], "always_notify": bool, "passive": bool}` | the source |
| `POST /v1/sources/kind/{kind}/mute` | same body as a source mute | `{kind, muted, muted_until}` |
| `DELETE /v1/sources/kind/{kind}/mute` | | same |
| `GET /v1/settings/quiet-hours` | | `{enabled, start, end, timezone, active_now}` |
| `PUT /v1/settings/quiet-hours` | any subset of `{"enabled": bool, "start": "HH:MM", "end": "HH:MM"}` | same |

`GET /v1/sources` also returns `kind_mutes`: `{"sensor": {muted, muted_until},
"dashboard": {...}, "camera": {...}}`. A kind mute (0.4.1) silences every
source of that kind, including ones that have not fired yet, which is the
only way to mute dashboard rules before their first alert. It is checked
right after the per-source mute and is independent of it: unmuting a kind
leaves individual source mutes in place.

Policy fields:

- `repeats` (sensor sources): which Marshal `repeat` messages push. `none`,
  `critical` (default: only critical rules), or `all`. The first sighting
  always pushes unless muted or in quiet hours.
- `objects` (camera sources): push only when the review's objects include one
  of these labels (Frigate labels: `person`, `car`, `dog`, …). Empty means
  any object. Default empty.
- `always_notify`: exempt from quiet hours. Default false.
- `passive` (0.6.0): deliver as a passive notification. Default false. See
  **Interruption level** below.

Push decision, in order, for every alert event:

1. Is it a pushable event? First sighting: yes. Sensor repeat: by `repeats`.
   Anything else (resolved, ended, Frigate merges): no.
2. Muted, as a source or as a kind? No push. Expired mutes are ignored.
3. Camera `objects` filter fails? No push.
4. Quiet hours active and not `always_notify`? No push.

Nothing above changes what is *recorded*: every alert is stored regardless.
Since 0.7.0 step 2 and step 3 also keep the alert out of that person's own
history and stream (see **History**); it stays in `scope=all`.

### Interruption level (0.6.0)

Every push carries `aps.interruption-level`, which tells iOS how to present
it. It is chosen after the decision above and never changes it:

| Level | When | On the phone |
|---|---|---|
| `passive` | the source's policy has `passive: true` | Notification Center only: no banner, no sound, no screen wake |
| `time-sensitive` | critical severity, or any camera alert | banner and sound, and breaks through Focus |
| `active` | everything else | banner and sound |

`passive` wins: a camera or a critical rule set to passive is passive. A
passive push carries no `sound` but is still sent at APNs priority 10
(priority 5 is throttled and may never arrive) and still has
`mutable-content`, so the thumbnail is attached as usual. Quiet hours hold a
passive push like any other; set `always_notify` as well to have overnight
alerts waiting silently in the morning. Apple's `critical` level needs an
entitlement Apple must approve, which the app does not have, so it is never
sent. Each delivery is logged as `push sent` with its `level`.

`POST /v1/rules/{name}/mute` and `DELETE` remain as aliases for sources that
are Marshal rules; new clients should use `/v1/sources`.

## Rule actions (0.5.0)

A Marshal rule can notify *and* act: the nightlights alert on motion and
switch the light. Marshal 0.5.0 lets the action be switched off on its own
("tell me about motion here, but leave the light alone"), and a source whose
rule has an action reports it:

```json
{ "id": "nightlight_motion_hall", "kind": "sensor", "...": "...",
  "action": { "active": true, "switchable": true, "owner": "automation" } }
```

- `active`: whether Marshal will start new cycles. It is the retained value
  on the rule's `active_topic`, else the configured `action.active` (default
  true), which is exactly how Marshal resolves it.
- `switchable`: the rule has an `active_topic`. Without one only the config
  can change `active`.
- `owner`: Marshal's `state_topic` value (`automation`, `override`,
  `parked`), absent until Marshal has published one. For context only: a
  parked or overridden action does nothing whatever `active` says.

`PUT /v1/sources/{id}/action` `{"active": true|false}` publishes `true` or
`false` to the `active_topic`, retained, and returns the source. `404` for an
unknown source, `409` when the rule has no `active_topic`, `502` when the
publish fails (nothing is recorded in that case).

Nothing is stored in sentinel. The retained message is the state, shared with
Marshal and with anything else that publishes there (`mosquitto_pub -r`, a
dashboard control), so sentinel mirrors it rather than owning it. Clearing the
retained value (`mosquitto_pub -r -n -t <active_topic>`) hands the decision
back to the config; the API has no call for that.

Inactive is not parked, and not muted:

| | stops pushes | stops the light | scope |
|---|---|---|---|
| mute (`/v1/sources/{id}/mute`) | yes | no | one source, or a kind |
| inactive (`/v1/sources/{id}/action`) | no | new on-commands only; a running cycle finishes | one rule |
| parked (`/v1/rules/{name}/enable`) | no | yes, and a light that is on stays on | every rule sharing the `enable_topic` |

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
| `API_TOKEN` | required | ≥16 chars; the owner's token, and the media-signing key |
| `API_PERSON` | `owner` | the owner's id |
| `API_PEOPLE` | unset | more people, `id:token,id:token` |
| `DB_PATH` | `/data/sentinel.db` | |
| `RETENTION` | `720h` | |
| `FRIGATE_URL` | `http://192.168.1.156:5000` | |
| `RULES_PATH` | `/etc/marshal/rules.yaml` | |
| `APNS_KEY_B64`, `APNS_KEY_ID`, `APNS_TEAM_ID`, `APNS_BUNDLE_ID` | unset | all four required for push; otherwise the server runs with push disabled |
| `DASHBOARD_URL`, `DASHBOARD_API_KEY` | unset | both required for the dashboard feed; otherwise it is off |
| `DASHBOARD_PUBLIC_URL` | unset | base of the deep links the phone opens |
| `DASHBOARD_LINK_USER_ID` | unset | appended to links as `user_id`, for anyone who has not set their own with `PUT /v1/me` |
| `DASHBOARD_MARK_SEEN` | `true` | app resolve → dashboard seen |

## Deploy

CI builds `ghcr.io/trv-enterprises/sentinel:<version>` from a tag:

```bash
git tag -a sentinel/v0.1.0 -m "sentinel v0.1.0" && git push origin sentinel/v0.1.0
```

Then from `homelab-deploy`: `make deploy-sentinel SENTINEL_VERSION=0.1.0`.
