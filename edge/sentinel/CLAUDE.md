# sentinel

Go service: Marshal + Frigate alerts -> SQLite -> APNs push + HTTP API for
the trv-sentinel iOS app. Read `README.md` for the API and config.

## Things that are deliberate

- **Persistent MQTT session, fixed client id.** `CleanSession(false)`, id
  `sentinel`, QoS 1 subscriptions, so Mosquitto queues alerts while the
  container restarts. This is the opposite of Marshal's unique-id rule, and on
  purpose: Marshal rebuilds its client on recovery and would evict itself;
  sentinel restarts in place as a single compose instance. Never run two
  sentinels against one broker. Liveness is a self-published
  `sentinel/heartbeat` loopback, not `IsConnected()`.
- **Camera alerts are created on the first message with
  `after.severity == "alert"`, whatever its `type`.** Frigate opens most
  reviews as `detection` and promotes them on `update`; filtering on
  `type == "new"` (what the kiosk does) misses those.
- **Sensor alerts are keyed by rule name while active.** Marshal's payload
  has no id. An orphan `repeat` creates (sentinel may have missed the `new`);
  an orphan `resolved` is ignored.
- **Stale sweep.** Marshal's state is in-memory; a condition that clears while
  Marshal is down never produces `resolved`. Active sensor alerts quiet for
  `max(2 × repeat_minutes, 6h)` go `stale`. `repeat_minutes` comes from the
  bind-mounted `rules.yaml`, which is why the mount exists in phase 1.
- **Media is signed, not bearer-authed.** AVPlayer, AsyncImage and the
  notification extension cannot reliably send headers. Signed links are
  `hmac(API_TOKEN, path|exp)`, 12h TTL, accepted only under `/v1/media/`.
  The proxy is `httputil.ReverseProxy` so `Range` and 206s just work.
- **Two APNs clients.** The environment is a property of the device token:
  Xcode-installed builds get sandbox tokens, distribution builds get
  production ones. The app reports `env` on registration; the server routes
  per device and prunes on `410` / `BadDeviceToken`.
- **Push policy:** first sighting of any alert; `repeat` only when critical;
  server-side mutes suppress.
- **Notifications are controlled per *source*, not per rule.** A source is a
  Marshal rule with an `alert:` block or a Frigate camera, id = the alert's
  `rule` field (`frigate_<camera>` for cameras). Action-only rules never
  notify and are not sources; nor is a rule with `alert.active: false`. The
  nightlights have had an `alert:` block since 2026-10, so they *are*
  sources, each with an `action` block (below). `internal/policy.Decider`
  is the single push decision: pushable event → mute → camera object filter
  → quiet hours (server-local time, `TZ` in compose). Policies and quiet
  hours live in SQLite; cameras come from Frigate's `/api/config` (cached
  10m) plus any camera seen in stored alerts.
- **Kind mutes live in the same `mutes` table** under the reserved key
  `kind:<kind>` (`policy.KindMuteKey`). Source ids never contain `:`, so no
  migration and no second table; `Mutes()` prunes expired ones the same way.
  The decider checks source then kind, so the app's "Mute all dashboard"
  covers rules that have not fired yet.
- **Mute ≠ enable.** Mute lives in sentinel's `mutes` table and only stops
  pushes. Enable/disable publishes to the rule's Marshal `enable_topic`,
  which parks the *automation* for every rule sharing that topic (the five
  nightlights share one). `false` is published retained so a Marshal restart
  stays parked; `true` is sent non-retained and the retained value cleared,
  because Marshal re-applies retained enables on reconnect and a retained
  `true` would wipe manual overrides each time. Owner state comes from
  subscribing to every `state_topic` in rules.yaml (retained, so warm on
  connect; re-subscribed each heartbeat for new rules).
  Known noise: the retained clear is an empty payload, which Marshal logs
  as `unparseable enable payload` once per rule sharing the topic. Harmless;
  the clean fix is for Marshal to ignore empty payloads silently (trv-marshal
  change, pending).
- **Action active is mirrored, never stored.** Marshal 0.5.0's `active_topic`
  holds a retained true/false that both services read; with nothing retained
  both fall back to `action.active` in rules.yaml. Marshal reports it nowhere
  (`state_topic` is ownership only), so there is no ack to wait for and no
  table: `PUT /v1/sources/{id}/action` publishes retained and
  `Engine.PublishRetained` records the value on the PUBACK, which is why the
  response is right without the `time.Sleep` that enable needs. Publish
  `true` retained too, do not clear it the way enable does: `SetActive` is a
  no-op for a value Marshal already holds, so a replayed `true` is harmless,
  and an explicit value survives a later change of the YAML default.
  `rules.ParseActive` must accept exactly the words Marshal's `parseActive`
  does, or the app shows a switch position Marshal is not in.
- **Commit, then push, then SSE.** A notification tap must never 404.
- **Dashboard alerts are pulled from the dashboard, not re-sunk from ts-store.**
  A ts-store rule has exactly one sink; pointing rules at MQTT would take them
  away from the bell and lose the dashboard link metadata. The dashboard's
  SSE feed already carries `dashboard_id`/`dashboard_vars`, so sentinel is a
  second subscriber with a view-only key. The feed has no replay and the
  only list is "unseen or pinned", hence backfill on (re)connect and a
  5-minute reconcile that resolves what was dismissed in the bell. The
  dashboard alert id lives in the `review_id` column (unique external id
  for every kind); `device` holds the human rule name, `rule` the source id.

## Local run

```bash
API_TOKEN=... MQTT_BROKER=tcp://<services-tailscale-ip>:1883 \
FRIGATE_URL=http://<nvr-tailscale-ip>:5000 RULES_PATH=/path/to/rules.yaml \
DB_PATH=./dev.db go run .
make pub-sensor BROKER=<services-tailscale-ip>   # needs mosquitto_pub
make alerts TOKEN=...
```

Without `mosquitto_pub` locally, publish from the broker container:
`docker compose exec mosquitto mosquitto_pub -h localhost -t sensors/alerts -m '...'`.

## Release

Tag `sentinel/vX.Y.Z`; CI vets, tests, builds multi-arch, pushes to GHCR.
First publish needs the GHCR package's Actions access granted to this repo
(see the root CLAUDE.md). Deploy with `make deploy-sentinel` in homelab-deploy.
