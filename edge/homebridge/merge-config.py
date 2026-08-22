#!/usr/bin/env python3
"""Merge repo accessories into Homebridge's LIVE config.

The repo's config.json is an accessory-only FRAGMENT: it carries just an
`accessories` array, never `bridge` or `platforms`. Those are host state --
the HomeKit pairing PIN, the bridge's MAC identity, and the Config UI
platform -- and they stay on the host so a public repo holds no secrets and
no one can overwrite a live config with a half-populated copy.

This merges accessories by name into the live config, leaving everything
else in the live file untouched.

Usage: merge-config.py <live.json> <repo.json> <out.json>
"""
import collections
import json
import sys


def main():
    if len(sys.argv) != 4:
        sys.exit(__doc__)
    live_path, repo_path, out_path = sys.argv[1:]

    load = lambda p: json.load(open(p), object_pairs_hook=collections.OrderedDict)
    live, repo = load(live_path), load(repo_path)

    if "bridge" in repo or "platforms" in repo:
        sys.exit(
            "repo config must be accessory-only: found "
            + ", ".join(k for k in ("bridge", "platforms") if k in repo)
            + " -- those are host state, not repo state"
        )

    accessories = live.setdefault("accessories", [])
    index = {a["name"]: i for i, a in enumerate(accessories)}

    added, updated = [], []
    for acc in repo.get("accessories", []):
        name = acc["name"]
        if name not in index:
            accessories.append(acc)
            added.append(name)
        elif accessories[index[name]] != acc:
            accessories[index[name]] = acc
            updated.append(name)

    # Guard: the merged result must still be a complete, usable config.
    # A live file missing its bridge section means we read the wrong file.
    pin = live.get("bridge", {}).get("pin", "")
    if not pin:
        sys.exit("refusing to write: live config has no bridge.pin")
    if pin.startswith("XXX"):
        sys.exit("refusing to write: live config has a placeholder PIN")

    with open(out_path, "w") as fh:
        json.dump(live, fh, indent=4)
        fh.write("\n")

    print("added:", ", ".join(added) or "(none)")
    print("updated:", ", ".join(updated) or "(none)")
    print("total accessories:", len(accessories))


if __name__ == "__main__":
    main()
