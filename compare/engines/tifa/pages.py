# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Page tables and archive: pages.py <run dir> --output <results/run-pages>.

Pages are compared after the router's update times (`u:` and 13 digits, the
Date.now() of the render) are masked: they differ between any two renders.
Times are per render: the mean of the CPU time of the timed renders where
the engine reports it, else the mean wall time (the C QuickJS rows, timed in
the script on a CPU of their own). Ratios are against liustack-quickjs on the
same page, as a geometric mean over the pages with the range in brackets.

liustack-tifa is slowjs set up the way tifa sets it up (own Intl and Temporal,
tifa's Go functions, no polyfill), so it has no pf runs. Its pages are
compared with the pages of liustack-quickjs on the polyfill, where the Intl
output of the two may differ in U+202F (see tifa_row_check).
"""
import glob
import argparse
import contextlib
import hashlib
import io
import json
import math
import os
import re
import sys
from pathlib import Path

GO = ["liustack-quickjs", "liustack-tifa", "modernc-quickjs", "go-quickjs-interp", "go-quickjs-jit", "goant-interp", "goant-jit", "moejs"]
REF = ["quickjs-2026-06-04", "quickjs-master", "quickjs-ng", "node-jit", "node-jitless"]
OWN_INTL = {"liustack-quickjs", "liustack-tifa", "go-quickjs-interp", "go-quickjs-jit", "node-jit", "node-jitless"}
LABEL = {
    "quickjs-2026-06-04": "C QuickJS 2026-06-04 (reference)",
    "quickjs-master": "C QuickJS master 7d1c13c5d8 (reference)",
    "quickjs-ng": "QuickJS-NG v0.17.0 (reference)",
    "node-jit": "Node v24.11.1 (reference)",
    "node-jitless": "Node v24.11.1 --jitless (reference)",
}

# Runs left out of run.sh, and why.
NOT_RUN = {("liustack-tifa", "pf"): "not run: this row has no polyfill", ("liustack-tifa", "mui-pf"): "not run: this row has no polyfill"}

out = None
mask = lambda b: re.sub(rb"\bu:\d{13}\b", b"u:0", b)


def load(name):
    p = os.path.join(out, name + ".json")
    if not os.path.exists(p) or os.path.getsize(p) == 0:
        return None
    try:
        return json.loads(Path(p).read_text())
    except ValueError:
        return {"err": "no result (the process did not finish)"}


def load_merged(name):
    """A run, with pages that failed taken from <name>-rest when there is one:
    a run without the pages that stopped the engine, in a process of its
    own (an engine that fails one page can leave its VM unusable)."""
    r = load(name)
    rest = load(name + "-rest")
    if not r or not rest:
        return r
    got = pages(rest)
    r = dict(r)
    r["pages"] = [got.get(pg["input"][:-5], pg) if not pg.get("ok") else pg for pg in r.get("pages") or []]
    # The pages left out of the rest run: the ones that stop the engine.
    r["failed"] = [pg["input"][:-5] for pg in load(name)["pages"] if not pg.get("ok") and pg["input"][:-5] not in got]
    return r


def meta(name):
    p = os.path.join(out, name + ".meta")
    if not os.path.exists(p):
        return []
    return [json.loads(l) for l in Path(p).read_text().splitlines() if l.strip()]


def html(engine, tag, page):
    p = os.path.join(out, "html", f"{engine}-{tag}-{page}.html")
    return mask(Path(p).read_bytes()) if os.path.exists(p) else None


def equality(actual, reference):
    return {
        "normalized_sha256": hashlib.sha256(actual).hexdigest() if actual is not None else None,
        "reference_sha256": hashlib.sha256(reference).hexdigest() if reference is not None else None,
        "same_as_liustack": actual == reference if actual is not None and reference is not None else None,
    }


def comparisons(kind, base_kind=None):
    result = {}
    for engine in GO + REF:
        k = kind(engine) if callable(kind) else kind
        base = base_kind or k
        ps = pages(load_merged(f"{engine}-{k}"))
        refs = pages(load_merged(f"liustack-quickjs-{base}"))
        result[engine] = {
            pg: equality(html(engine, k, pg) if ps.get(pg, {}).get("ok") else None,
                         html("liustack-quickjs", base, pg) if refs.get(pg, {}).get("ok") else None)
            for pg in sorted(ps.keys() | refs.keys())
        }
    return result


TIFA_KINDS = [("bench", "bench", "bench"), ("mui", "mui-native", "mui-pf"), ("shop", "native", "pf")]
NARROW_SPACE = "\u202f".encode()


def tifa_row_evidence():
    """The pages of liustack-tifa against those of liustack-quickjs on the
    polyfill (the same JavaScript on another Intl). `same` is byte for byte
    after masking. `same_but_u202f` says the pages are the same once the
    narrow no-break space U+202F, which the FormatJS polyfill writes before
    AM and PM where Go's Intl and Node write a plain space, is a plain space."""
    result = {}
    for app, tifa_kind, pf_kind in TIFA_KINDS:
        ps = pages(load_merged(f"liustack-tifa-{tifa_kind}"))
        refs = pages(load_merged(f"liustack-quickjs-{pf_kind}"))
        for pg in sorted(ps.keys() | refs.keys()):
            a = html("liustack-tifa", tifa_kind, pg) if ps.get(pg, {}).get("ok") else None
            b = html("liustack-quickjs", pf_kind, pg) if refs.get(pg, {}).get("ok") else None
            e = equality(a, b)
            same = e.pop("same_as_liustack")
            e["same"] = same
            e["same_but_u202f"] = (a.replace(NARROW_SPACE, b" ") == b.replace(NARROW_SPACE, b" ")) if a is not None and b is not None else None
            e["u202f_in_polyfill_page"] = b.count(NARROW_SPACE) if b is not None else None
            e["u202f_in_tifa_page"] = a.count(NARROW_SPACE) if a is not None else None
            result[f"{app}/{pg}"] = e
    return result


def check_equality(app, page):
    def read(tag):
        p = os.path.join(out, "check", f"liustack-quickjs-{app}-{tag}-{page}.html")
        return mask(Path(p).read_bytes()) if os.path.exists(p) else None
    return equality(read("stub"), read("normal"))


def archive():
    # Keep every process result, including rest runs, single-page probes and
    # metadata for processes that did not produce JSON. Never replace raw rows.
    names = {os.path.relpath(p, out).rsplit(".", 1)[0]
             for ext in ("json", "meta")
             for p in glob.glob(os.path.join(out, "**", "*." + ext), recursive=True)}
    result = {name: {"result": load(name), "tries": meta(name)} for name in sorted(names)}
    result["_html"] = {
        "normalization": {"version": 1, "pattern": r"\bu:\d{13}\b", "replacement": "u:0", "digest": "sha256"},
        "comparisons": {
            "bench": comparisons("bench"),
            "pf": comparisons("pf"),
            "deployed": comparisons(lambda e: "native" if e in OWN_INTL else "pf", "native"),
            "mui_pf": comparisons("mui-pf"),
            "mui_deployed": comparisons(lambda e: "mui-native" if e in OWN_INTL else "mui-pf", "mui-native"),
        },
        "tifa_vs_polyfill": tifa_row_evidence(),
        "checks": {app: {pg: check_equality(app, pg) for pg in sorted(pages(load(f"check/{app}-normal")))}
                   for app in ("bench", "mui", "shop")},
    }
    return result


def per_render(p):
    if not p.get("ok"):
        return None
    if p.get("cpu_ms_per_op"):
        return p["cpu_ms_per_op"]
    return p["render"]["mean_ns"] / 1e6


def pages(r):
    return {pg["input"][:-5]: pg for pg in (r or {}).get("pages") or []}


def gm(xs):
    return math.exp(sum(math.log(x) for x in xs) / len(xs))


def f(x):
    return "" if x is None else (f"{x:.3g}" if x < 100 else f"{x:.0f}")


def ratio_cell(rows, base):
    rs = [rows[k] / base[k] for k in rows if k in base and rows[k] and base[k]]
    if not rs:
        return ""
    return f"{gm(rs):.2f} ({min(rs):.2f}–{max(rs):.2f})"


def why(r):
    if r is None:
        return "not run"
    if r.get("err"):
        return r["err"].split("\n")[0][:160]
    bad = [f"{k}: {v.get('err', '')[:120]}" for k, v in pages(r).items() if not v.get("ok")]
    return "; ".join(bad[:3])


def disturbed(name):
    m = meta(name)
    return m and m[-1].get("disturbed")


def check():
    print("### Pages without Intl\n")
    print("| App | Page | Same page with Intl removed | Intl it reaches (first caller) |")
    print("|---|---|---|---|")
    for app in ["bench", "mui", "shop"]:
        n = load(f"check/{app}-normal")
        s = load(f"check/{app}-stub") or {}
        for page in sorted(pages(n)):
            if s.get("err"):
                same = "no: " + s["err"][:80]
            else:
                same = {True: "yes", False: "no", None: "not archived"}[check_equality(app, page)["same_as_liustack"]]
            pr = load(f"check/{app}-proxy-{page}") or {}
            hits = json.loads((pages(pr).get(page) or {}).get("probe") or "{}")
            reach = ", ".join(f"`{k}` ×{v['n']}" for k, v in hits.items()) or "nothing"
            print(f"| {app} | {page} | {same} | {reach} |")
    print()


def table(kind, title, engines, base_kind=None, tag=None):
    tag = tag or kind
    base = {k: per_render(v) for k, v in pages(load_merged(f"liustack-quickjs-{base_kind or kind}")).items()}
    evidence = comparisons(kind, base_kind)
    print(f"### {title}\n")
    print("| Engine | Load ms | Pages that render | Same page as liustack-quickjs | Time per render vs liustack-quickjs |")
    print("|---|---|---|---|---|")
    for e in engines:
        k = kind(e) if callable(kind) else kind
        r = load_merged(f"{e}-{k}")
        name = LABEL.get(e, e)
        if callable(kind):
            name += f" ({'own Intl' if k == 'native' else 'polyfill'})"
        if r is None:
            print(f"| {name} | {NOT_RUN.get((e, k), 'not run')} | | | |")
            continue
        ps = pages(r)
        rows = {pg: per_render(v) for pg, v in ps.items() if per_render(v)}
        if not rows:
            print(f"| {name} | {why(r)} | 0 of {len(base)} | | |")
            continue
        same = sum(1 for pg in rows if evidence[e][pg]["same_as_liustack"] is True)
        missing = sum(1 for pg in rows if evidence[e][pg]["same_as_liustack"] is None)
        load_ms = (r.get("load_ms") or 0) + sum(r.get("prelude_ms") or [])
        note = " (disturbed)" if disturbed(f"{e}-{k}") else ""
        if missing:
            note += f". HTML unavailable for {missing} pages"
        if r.get("failed"):
            fails = r["failed"]
            if len(fails) > len(base) / 2:
                which = "every page but " + ", ".join(sorted(set(base) - set(fails))) + " fails"
            else:
                which = ", ".join(fails) + " fail"
            note += f". {which}, a failure leaves the runtime closed, the other pages measured in a run without the failing ones"
        ok = sum(1 for pg in ps.values() if pg.get("ok"))
        print(f"| {name} | {load_ms:.0f} | {ok} of {len(base)} | {same} of {len(rows)} | {ratio_cell(rows, base)}{note} |")
    print()


def per_page(kind, engines, base_kind=None):
    base = {k: per_render(v) for k, v in pages(load(f"liustack-quickjs-{base_kind or kind}")).items()}
    print("| Page | " + " | ".join(engines) + " |")
    print("|---|" + "---|" * len(engines))
    rs = {e: pages(load_merged(f"{e}-{kind(e) if callable(kind) else kind}")) for e in engines}
    for pg in sorted(base):
        cells = []
        for e in engines:
            v = rs[e].get(pg)
            x = per_render(v) if v else None
            cells.append("" if x is None else f"{f(x)} ({x / base[pg]:.2f})")
        print(f"| {pg} | " + " | ".join(cells) + " |")
    print()


def tifa_row_check():
    print("### liustack-tifa against the same JavaScript on the polyfill\n")
    print("| Bundle | Pages | Same HTML | Same but for U+202F | Other difference | Not compared |")
    print("|---|---|---|---|---|---|")
    ev = tifa_row_evidence()
    for app, _, _ in TIFA_KINDS:
        rows = {k: v for k, v in ev.items() if k.startswith(app + "/")}
        same = sum(1 for v in rows.values() if v["same"] is True)
        u202f = sum(1 for v in rows.values() if v["same"] is False and v["same_but_u202f"] is True)
        other = sum(1 for v in rows.values() if v["same"] is False and not v["same_but_u202f"])
        missing = sum(1 for v in rows.values() if v["same"] is None)
        print(f"| {app} | {len(rows)} | {same} | {u202f} | {other} | {missing} |")
    print()


def vs_c():
    """Per bundle, every row as a ratio to C QuickJS 2026-06-04 on the same pages."""
    pf = {"bench": "bench", "mui": "mui-pf", "shop": "pf"}
    own = {"bench": "bench", "mui": "mui-native", "shop": "native"}
    rows = [
        ("liustack-quickjs, same JavaScript (polyfill)", "liustack-quickjs", pf),
        ("liustack-quickjs, own Intl only", "liustack-quickjs", own),
        ("liustack-tifa, as tifa runs it", "liustack-tifa", own),
        ("C QuickJS 2026-06-04", "quickjs-2026-06-04", pf),
        ("Node v24.11.1", "node-jit", pf),
        ("Node v24.11.1 --jitless", "node-jitless", pf),
    ]
    print("### Per bundle, against C QuickJS 2026-06-04\n")
    print("Geometric mean over the pages of a bundle, range in brackets. Below 1 is faster than C. Bench has no Intl, MUI and the shop need it: every row but liustack-tifa and the \"own Intl only\" row runs them on the polyfill.\n")
    print("| Row | bench | MUI | shop |")
    print("|---|---|---|---|")
    for label, engine, kinds in rows:
        cells = []
        for app in ["bench", "mui", "shop"]:
            base = {k: per_render(v) for k, v in pages(load_merged(f"quickjs-2026-06-04-{pf[app]}")).items()}
            mine = {k: per_render(v) for k, v in pages(load_merged(f"{engine}-{kinds[app]}")).items() if per_render(v)}
            cells.append(ratio_cell(mine, base) if base and mine else "")
        print(f"| {label} | " + " | ".join(cells) + " |")
    print()


def polyfill_cost():
    print("### What the polyfill costs liustack-quickjs\n")
    pf, nat = load("liustack-quickjs-pf"), load("liustack-quickjs-native")
    print(f"- Loading it: {sum(pf['prelude_ms']):.0f} ms per VM, before the bundle ({pf['load_ms']:.0f} ms after it, {nat['load_ms']:.0f} ms without it).")
    for k in ["pf", "native"]:
        m = (load(f"liustack-quickjs-mem-{k}") or {}).get("mem") or {}
        g = (load(f"go-quickjs-interp-mem-{k}") or {}).get("mem") or {}
        print(f"- Memory per VM, {'polyfill' if k == 'pf' else 'own Intl'}: liustack-quickjs {m.get('rss_per_vm_bytes', 0) / 2**20:.1f} MiB RSS, go-quickjs-interp {g.get('rss_per_vm_bytes', 0) / 2**20:.1f} MiB RSS ({g.get('go_heap_per_vm_bytes', 0) / 2**20:.1f} MiB Go heap)")
    for k, what in [("timer-pf", "the polyfill"), ("timer-native", "its own Intl")]:
        r = load(f"liustack-quickjs-{k}")
        if not r:
            continue
        tot, ren, calls = 0.0, 0.0, 0
        for pg in r["pages"]:
            if not pg.get("ok"):
                continue
            t = json.loads(pg["probe_end"])
            n = pg["render"]["n"] + 20  # the probe covers the warm renders too
            tot += t["ms"] / n
            calls += t["calls"] / n
            ren += pg["cpu_ms_per_op"]
        print(f"- Time inside {what}: {tot:.1f} ms of {ren:.1f} ms per render summed over the pages ({100 * tot / ren:.0f}%), {calls:.0f} calls.")
    print()


def markdown():
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        check()
        engines = GO + REF
        table("bench", "Pages without Intl (bench)", engines)
        per_page("bench", engines)
        table("pf", "Shop on the FormatJS polyfill, every engine", engines)
        per_page("pf", engines)
        table(lambda e: "native" if e in OWN_INTL else "pf", "Shop as deployed: own Intl where the engine has one", engines, base_kind="native")
        per_page(lambda e: "native" if e in OWN_INTL else "pf", engines, base_kind="native")
        table("mui-pf", "MUI pages on the FormatJS polyfill, every engine", engines)
        per_page("mui-pf", engines)
        table(lambda e: "mui-native" if e in OWN_INTL else "mui-pf", "MUI pages as deployed: own Intl where the engine has one", engines, base_kind="mui-native")
        per_page(lambda e: "mui-native" if e in OWN_INTL else "mui-pf", engines, base_kind="mui-native")
        tifa_row_check()
        vs_c()
        polyfill_cost()
    return "\n".join(line.rstrip() for line in buf.getvalue().splitlines()).rstrip() + "\n"


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("run_dir")
    parser.add_argument("--output", help="write <stem>.md and <stem>.json outside the raw run directory")
    args = parser.parse_args()
    out = os.path.abspath(args.run_dir)
    if args.output and os.path.commonpath([out, os.path.abspath(args.output)]) == out:
        parser.error("--output must be outside the raw run directory")
    report = markdown()
    if args.output:
        data = archive()
        with open(args.output + ".json", "w") as stream:
            json.dump(data, stream, indent=2, sort_keys=True)
            stream.write("\n")
        with open(args.output + ".md", "w") as stream:
            stream.write(report)
    else:
        sys.stdout.write(report)
