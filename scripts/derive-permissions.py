#!/usr/bin/env python3
"""Derive the smallest permission catalogue that preserves today's behaviour.

Start coarse — one permission per module and action — and split a permission
only when the routes under it do not share a rank floor. A split that is not
forced is a row somebody has to understand in the admin grid for no reason; a
split that is skipped silently grants or removes access.

The result is checked both ways at the end: every route maps to exactly one
permission, and every permission covers routes of exactly one floor.
"""
import json
import re
from collections import defaultdict

SCRATCH = "/private/tmp/claude-501/-Users-sudipto-Desktop-projects-npdms/6795f28e-4b8d-4986-a2ad-60802678b8e5/scratchpad"
BY_METHOD = {"GET": "view", "POST": "create", "PUT": "amend", "PATCH": "amend", "DELETE": "delete"}
HIERARCHY = {
    "CONSTABLE": 1, "HEAD_CONSTABLE": 2, "ASI": 3, "SI": 4, "INSPECTOR": 5,
    "SHO": 6, "DSP": 7, "SP": 8, "DIG": 9, "IG": 10, "SECRETARY": 11, "DGP": 12,
}
SKIP = ("health", "ready", "openapi.yaml", "auth", "public")

routes = [r for r in json.load(open(f"{SCRATCH}/routes.json")) if r["module"] not in SKIP]

for r in routes:
    parts = [p for p in r["path"].replace("/api/v1", "").split("/") if p]
    r["parts"] = parts
    r["module"] = parts[0]
    # The named segments after the module, ignoring :id placeholders. These are
    # what a split can use to tell two routes apart.
    r["named"] = [p for p in parts[1:] if not p.startswith(":")]


def is_verb_tail(r):
    """Does the path end in something being done to a particular record?

    /malkhana/items/:id/reseal ends in a verb — the segment acts on the item
    named by :id. /malkhana/items ends in a noun: it is the collection, and
    what is being done to it comes from the method. The difference is whether
    a placeholder sits immediately before the last segment.
    """
    parts = r["parts"]
    if len(parts) < 2 or parts[-1].startswith(":"):
        return False
    # A GET never does anything: /items/:id/movements reads the movements, it
    # does not move anything, so it takes the .view action like any other read.
    if r["method"] == "GET":
        return False
    return parts[-2].startswith(":")


def name(r, depth):
    """module[.segment...].action — depth segments of detail."""
    bits = [r["module"]] + r["named"][:depth]
    # Every permission ends in something being done, so the grid reads as a
    # list of capabilities rather than a list of nouns.
    if is_verb_tail(r) and depth >= len(r["named"]):
        return ".".join(bits)
    return ".".join(bits + [BY_METHOD[r["method"]]])


# Assign each route the coarsest name that does not collide on floor.
assigned = {}
for r in routes:
    r["depth"] = 0

for _ in range(6):  # deepest path has ~4 named segments
    groups = defaultdict(list)
    for r in routes:
        groups[name(r, r["depth"])].append(r)
    changed = False
    for p, rs in groups.items():
        if len({x["floor"] for x in rs}) > 1:
            for x in rs:
                if x["depth"] < len(x["named"]):
                    x["depth"] += 1
                    changed = True
    if not changed:
        break

groups = defaultdict(list)
for r in routes:
    r["permission"] = name(r, r["depth"])
    groups[r["permission"]].append(r)

bad = {p: rs for p, rs in groups.items() if len({r["floor"] for r in rs}) > 1}

print(f"routes      : {len(routes)}")
print(f"permissions : {len(groups)}")
print(f"unresolved  : {len(bad)}")
for p, rs in list(bad.items())[:10]:
    print(f"   {p}: floors {sorted({r['floor'] or '-' for r in rs})}")
    for r in rs[:6]:
        print(f"      {r['floor'] or '-':14s} {r['method']:6s} {r['path']}")

by_module = defaultdict(set)
for p in groups:
    by_module[p.split(".")[0]].add(p)
print()
print(f"modules     : {len(by_module)}")
print("largest     :", ", ".join(f"{m}({len(s)})" for m, s in
                                 sorted(by_module.items(), key=lambda kv: -len(kv[1]))[:8]))

out = {}
for p, rs in sorted(groups.items()):
    floor = rs[0]["floor"]
    out[p] = {"floor": floor, "module": p.split(".")[0],
              "routes": [f"{r['method']} {r['path']}" for r in sorted(rs, key=lambda x: x["path"])]}
json.dump(out, open(f"{SCRATCH}/permissions.json", "w"), indent=1)

granted = sum(1 for p, v in out.items() if v["floor"] is None)
print(f"no floor today (any signed-in officer): {granted} of {len(out)}")
