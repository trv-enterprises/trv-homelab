# Motion Nightlight Automation

How the Third Reality 3RSNL02043Z night light is driven, and why it is driven
that way.

## The problem

The device can run motion→light entirely on its own. That on-board rule is
invisible to Zigbee2MQTT: it fires without announcing itself and cannot be
queried. If an external rule engine also drives the light, the two contend at
unpredictable moments and the light flickers. This is a race, not a conflict
that can be arbitrated after the fact — so exactly one of them must own the
device at any moment.

## Ownership

A strict precedence stack, highest first:

| Tier | Owner | Set by |
|---|---|---|
| 1 | **parked** | HomeKit "Nightlight Auto" switch off |
| 2 | **override** | any manual command, for `override_ttl_minutes` |
| 3 | **automation** | the `nightlight_motion` rule |
| 4 | **device-local** | the device itself, when the engine stays silent |

Tier 4 is not contended: it is what remains when the engine issues no commands.
It is kept dormant (`localRoutinTime = 0`) so it cannot race tiers 1–3.

The manual override is **TTL-based on purpose**. A permanent "force on" flag is
how a nightlight ends up on at noon; the TTL guarantees automation resumes on
its own. The Auto switch exists for the other direction — handing control back
immediately rather than waiting out the TTL.

## The on-device rule (private cluster 0xFC00)

Z2M defines these attributes but wires no converter to them, so they are
unreachable via `<device>/set` and must be written with the raw ZCL API:

| Attribute | ID | Meaning |
|---|---|---|
| `coldDownTime` | 0x0003 | cooldown before motion can retrigger (s) |
| `localRoutinTime` | 0x0004 | how long the light stays on after motion (s) |
| `luxThreshold` | 0x0005 | only run the local rule below this ambient level |

Manufacturer code `0x130D` (4877), cluster `0xFC00` (64512), endpoint 1.

**The device does not answer reads on this cluster.** Its configuration cannot
be queried back, so `inventory/host_vars/services.yml` is the source of truth
and the Ansible role re-asserts it on every deploy. That also repairs drift
after a power cycle or an OTA.

To hand control back to the device — e.g. so the light still works during a
broker or LXC outage — set `on_seconds` to a hold time and disable the engine
rule. That is a deliberate trade: the light keeps working standalone, but
HomeKit override no longer reliably wins.

## Self-override hazard

The rule's `override_topic` is the device's own `/set` topic, so that a command
from *any* source (HomeKit, dashboard, `mosquitto_pub`) counts as manual. The
engine therefore hears its own publishes. Those echoes are pre-registered and
consumed by the actuator so the engine does not override itself on its first
command and go permanently dormant. See `TestSelfCommandEchoIsNotAnOverride`.

## HomeKit surface

Three accessories, so that arbitration stays in the engine rather than being
smeared across HomeKit automations:

- **Nightlight** (lightbulb) — manual control; any change starts the override TTL.
- **Nightlight Motion** (occupancy sensor) — read-only.
- **Nightlight Auto** (switch) — parks/resumes automation; turning it on also
  clears an active override.

The Auto switch reads the engine's retained owner topic
(`automation/nightlight/owner`), which carries a bare string, not Z2M JSON —
the codec handles it before its JSON parse.

## Deploy

```bash
make deploy-alert-engine     # rules + on-device attribute write
```

Homebridge accessories deploy separately from `edge/homebridge/`.

## Verifying

```bash
# who owns the light right now
mosquitto_sub -h <broker> -t 'automation/nightlight/owner' -v

# take manual control (starts the TTL)
mosquitto_pub -h <broker> -t 'zigbee2mqtt/motion-night-light-001/set' -m '{"state":"ON"}'

# hand control straight back
mosquitto_pub -h <broker> -t 'automation/nightlight/enable' -m 'true'
```
