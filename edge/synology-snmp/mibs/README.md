# Synology MIB reference

Vendor MIB sources for the `synology-snmp` collector. Checked in because an
SNMP walk returns bare integers — `diskStatus = 1` carries no meaning on the
wire. Everything needed to interpret those integers lives ONLY in these files.

## Contents

| File | Source | Retrieved |
|------|--------|-----------|
| `SYNOLOGY-*-MIB.txt` (16 files) | Synology MIB bundle | dated 2022-05-04 |
| `MIB-Guide-2025-extract.txt` | text extract of the DiskStation MIB Guide PDF | 2026-08-01 |

MIB Guide PDF source URL (Synology rotates these; may 404 later):
`https://global.synologydownload.com/download/Document/Software/DeveloperGuide/Firmware/DSM/All/enu/Synology_DiskStation_MIB_Guide.pdf`

## ⚠️ The two sources disagree, and BOTH are needed

The checked-in `.txt` MIBs are the authority for **OID structure, types and
units**. They are NOT the authority for **enum ranges** — the bundle is from
2022 and Synology has extended enums since without reissuing it.

Verified example (`raidStatus`, OID .1.3.6.1.4.1.6574.3.1.1.3):

- 2022 MIB declares `SYNTAX Integer32(1..12)` and documents 12 states.
- 2025 PDF guide documents **20** states, including `DataScrubbing(13)`,
  `RaidExpandingUnfinishedSHR(18)`, `RaidConvertSHRToPool(19)` and
  `RaidMigrateSHR1ToSHR2(20)`.

This matters here specifically: this NAS runs **SHR-2**, so states 18/20 are
exactly what it reports during SHR maintenance. A decoder built from the MIB
alone would render those as out-of-range during the operation you most want
to watch.

`diskHealthStatus` shows the inverse gap: the MIB declares `Integer32(1..5)`
but only documents four values (Normal/Warning/Critical/Failing). Value 5 is
reserved-but-undescribed in both sources.

## Consequences for the collector

1. **Store the raw integer, never the decoded string.** Decoding is a
   read-time concern. A `string` field would need a retype every time
   Synology appends an enum value — and under ts-store's schema rules a
   retype means recreating the store and losing history. Integers absorb the
   drift for free.
2. **Treat every enum table as OPEN.** An unknown integer must pass through
   as its raw number, never error and never collapse to "unknown" — we have
   direct evidence (above) that Synology extends these tables silently.
3. **Apply the open-enum rule everywhere**, not just to disk/RAID. The other
   enums (systemStatus, powerStatus, fan) were not cross-checked against the
   PDF and are presumed to drift the same way.

## Field notes confirmed from these sources

- `diskTemperature` — **Celsius**, stated explicitly in SYNOLOGY-DISK-MIB.txt.
  (The PDF guide never states a unit; do not cite it for this.)
- `raidFreeSize` / `raidTotalSize` — `Counter64`, i.e. **bytes**.
- `upsInfoLoadValue` / `upsBatteryChargeValue` — `Float` (percent).
- `upsInfoStatus` — `DisplayString`, NOT an enum (e.g. `"OL"` = on line).
