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
  server-side mutes (phase 3) suppress.
- **Commit, then push, then SSE.** A notification tap must never 404.

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
