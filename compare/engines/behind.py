# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Items where slowjs needs at least 2x the time of the median of
the other pure Go engines. python3 behind.py <results json>"""
import json, statistics, sys
r = json.load(open(sys.argv[1]))
t = r["timed"]
# Results files made before 2026-10-24 name the row liustack-quickjs, the
# module's name until v0.28.0.
names = [e["engine"] for e in r["engines"]]
ME = "slowjs" if "slowjs" in names or "liustack-quickjs" not in names else "liustack-quickjs"
others = [e for e in names if e != ME]
med = statistics.median
rows = []
# QuickJS micro benchmarks: ns per iteration, median of the rounds
def up(e):
    out = {}
    for rd in t["upstream"].get(e, []):
        for k, v in (rd.get("tests") or {}).items():
            if "ns_per_n" in v:
                out.setdefault(k, []).append(v["ns_per_n"])
    return {k: med(v) for k, v in out.items()}
U = {e: up(e) for e in [ME] + others}
for k, mine in U[ME].items():
    xs = {e: U[e][k] for e in others if k in U[e]}
    if len(xs) < 2: continue
    m = med(xs.values())
    rows.append(("micro", k, mine / m, mine, m, xs))
# V8: score, higher is better
def v8(e):
    out = {}
    for rd in t["v8"].get(e, []):
        for k, v in (rd.get("scores") or {}).items():
            try: out.setdefault(k, []).append(float(v))
            except ValueError: pass
    return {k: med(v) for k, v in out.items()}
V = {e: v8(e) for e in [ME] + others}
for k, mine in V[ME].items():
    xs = {e: V[e][k] for e in others if k in V[e]}
    m = med(xs.values())
    rows.append(("v8", k, m / mine, mine, m, xs))
for w in ["react", "micro/json-parse", "micro/json-stringify", "micro/regexp", "micro/string-concat"]:
    def cpu(e):
        v = [x["cpu_ms_per_op"] for x in t[w].get(e, []) if x.get("ok")]
        return med(v) if v else None
    mine = cpu(ME)
    xs = {e: cpu(e) for e in others if cpu(e)}
    m = med(xs.values())
    rows.append(("workload", w, mine / m, mine, m, xs))
rows.sort(key=lambda x: -x[2])
lim = float(sys.argv[2]) if len(sys.argv) > 2 else 2.0
for kind, k, ratio, mine, m, xs in rows:
    if ratio >= lim:
        best = min(xs, key=xs.get) if kind != "v8" else max(xs, key=xs.get)
        print(f"{kind:8} {k:28} {ratio:5.2f}  ours {mine:.4g}  median {m:.4g}  best {best} {xs[best]:.4g}  n={len(xs)}")
