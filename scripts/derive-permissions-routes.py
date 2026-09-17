#!/usr/bin/env python3
"""Extract every route and its current rank floor from main.go.

The output is the raw material for the permission catalogue: what exists, what
guards it today, and therefore what the seeded per-rank roles must grant if
behaviour is to be preserved exactly.
"""
import json
import re
import sys
from collections import defaultdict

MAIN = "/Users/sudipto/Desktop/projects/npdms-backend/services/api/main.go"
HIERARCHY = {
    "CONSTABLE": 1, "HEAD_CONSTABLE": 2, "ASI": 3, "SI": 4, "INSPECTOR": 5,
    "SHO": 6, "DSP": 7, "SP": 8, "DIG": 9, "IG": 10, "SECRETARY": 11, "DGP": 12,
}

src = open(MAIN).read()
lines = src.split("\n")

# Group prefixes: `x := parent.Group("/path")` and `{ ... }` nesting. Rather
# than parse Go, track brace depth and the most recent Group() at each depth.
route_re = re.compile(r'(\w+)\.(GET|POST|PUT|PATCH|DELETE)\(\s*"([^"]*)"(.*)$')
group_re = re.compile(r'(\w+)\s*:=\s*(\w+)\.Group\(\s*"([^"]*)"')
use_re = re.compile(r'(\w+)\.Use\(\s*middleware\.RequireRole\(([^)]*)\)')
prefix = {"router": "", "v1": "/api/v1", "protected": "/api/v1"}
group_floor = {}

routes = []
for line in lines:
    m = group_re.search(line)
    if m:
        child, parent, path = m.group(1), m.group(2), m.group(3)
        prefix[child] = prefix.get(parent, "") + path
        if parent in group_floor:
            group_floor[child] = group_floor[parent]
        continue

    m = use_re.search(line)
    if m:
        var, args = m.group(1), m.group(2)
        ranks = re.findall(r'"([A-Z_]+)"', args)
        if ranks:
            group_floor[var] = min(ranks, key=lambda r: HIERARCHY.get(r, 0))
        continue

    m = route_re.search(line)
    if m:
        var, method, path, rest = m.group(1), m.group(2), m.group(3), m.group(4)
        ranks = re.findall(r'RequireRole\(([^)]*)\)', rest)
        floor = None
        if ranks:
            names = re.findall(r'"([A-Z_]+)"', ranks[0])
            if names:
                floor = min(names, key=lambda r: HIERARCHY.get(r, 0))
        if floor is None:
            floor = group_floor.get(var)
        full = prefix.get(var, "?" + var) + path
        routes.append({"method": method, "path": full, "floor": floor, "group": var})

print(f"routes found: {len(routes)}", file=sys.stderr)
unknown = [r for r in routes if r["path"].startswith("?")]
print(f"unresolved group prefix: {len(unknown)}", file=sys.stderr)
for r in unknown[:5]:
    print("   ", r, file=sys.stderr)

guarded = [r for r in routes if r["floor"]]
print(f"with a rank floor: {len(guarded)}  without: {len(routes) - len(guarded)}", file=sys.stderr)

# Module = the first segment after /api/v1.
by_module = defaultdict(list)
for r in routes:
    parts = [p for p in r["path"].replace("/api/v1", "").split("/") if p]
    module = parts[0] if parts else "root"
    r["module"] = module
    by_module[module].append(r)

print(f"modules: {len(by_module)}", file=sys.stderr)
json.dump(routes, open("/private/tmp/claude-501/-Users-sudipto-Desktop-projects-npdms/6795f28e-4b8d-4986-a2ad-60802678b8e5/scratchpad/routes.json", "w"), indent=1)
for module in sorted(by_module, key=lambda m: -len(by_module[m])):
    rs = by_module[module]
    floors = sorted({r["floor"] or "-" for r in rs}, key=lambda r: HIERARCHY.get(r, 0))
    print(f"{len(rs):4d}  {module:28s} {','.join(floors)}")
