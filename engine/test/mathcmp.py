#!/usr/bin/env python3
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Compare two outputs of mathdump, libmath or libmdump, one result at a time.

    mathcmp.py --count N --names "NAME NAME..." FIRST.txt SECOND.txt

Each file has, for every function, an "in" line with the bits of the arguments
and an "out" line with the bits of the results:

    in NAME N DATA
    out NAME N DATA

DATA is N groups of 16 hex digits per argument or result. The files are
refused unless they hold exactly the functions named, each with one "in" and
one "out" line, with N entries as --count says, in hex digits only, with the
same inputs and the same width of result in both. Then the results are
compared. Prints, for each function, how many were compared and how many
differ, then the first few that differ. Exits 1 when any differs, 2 when a file
is refused.
"""
import argparse
import re
import sys

HEX = re.compile(r"^[0-9a-f]+$")


def refuse(msg):
    print("mathcmp: " + msg, file=sys.stderr)
    sys.exit(2)


def read(path, names, count):
    rows = {}
    with open(path) as f:
        for lineno, line in enumerate(f, 1):
            fields = line.rstrip("\n").split(" ")
            if len(fields) != 4:
                refuse(f"{path}:{lineno}: want 'KIND NAME N DATA'")
            kind, name, n, data = fields
            if kind not in ("in", "out"):
                refuse(f"{path}:{lineno}: kind {kind!r}")
            if name not in names:
                refuse(f"{path}:{lineno}: unexpected function {name!r}")
            if (kind, name) in rows:
                refuse(f"{path}:{lineno}: {kind} {name} twice")
            if n != str(count):
                refuse(f"{path}:{lineno}: {kind} {name} has {n} entries, want {count}")
            if not data or not HEX.match(data):
                refuse(f"{path}:{lineno}: {kind} {name}: the data is empty or not lower case hex")
            if len(data) % (16 * count):
                refuse(f"{path}:{lineno}: {kind} {name}: {len(data)} digits are not {count} whole entries")
            rows[(kind, name)] = data
    for name in names:
        for kind in ("in", "out"):
            if (kind, name) not in rows:
                refuse(f"{path}: no {kind} line for {name}")
    return rows


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--count", type=int, required=True)
    ap.add_argument("--names", required=True)
    ap.add_argument("first")
    ap.add_argument("second")
    a = ap.parse_args()
    names = a.names.split()
    if not names or len(set(names)) != len(names) or a.count <= 0:
        refuse("--names and --count must name some functions once each and a count over 0")
    x, y = read(a.first, names, a.count), read(a.second, names, a.count)
    total = bad = 0
    for name in names:
        if x[("in", name)] != y[("in", name)]:
            refuse(f"{name}: the inputs differ, so the results cannot be compared")
        ox, oy = x[("out", name)], y[("out", name)]
        w = len(ox) // a.count
        if len(oy) != len(ox):
            refuse(f"{name}: the results have different widths ({len(ox)} and {len(oy)} digits)")
        wi = len(x[("in", name)]) // a.count
        diff = [i for i in range(a.count) if ox[i * w:(i + 1) * w] != oy[i * w:(i + 1) * w]]
        total += a.count
        bad += len(diff)
        print(f"{name:8} {a.count} compared, {len(diff)} differ")
        for i in diff[:5]:
            args = " ".join(x[("in", name)][i * wi + j:i * wi + j + 16] for j in range(0, wi, 16))
            print(f"    input {i}: {args}  first {ox[i * w:(i + 1) * w]}  second {oy[i * w:(i + 1) * w]}")
    print(f"{total} results compared, {bad} differ")
    return 1 if bad else 0


sys.exit(main())
