#!/usr/bin/env python3
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Compare two outputs of mathdump, one result at a time.

    mathcmp.py NATIVE.txt GO.txt

Each file has, for every function, an "in" line with the bits of the arguments
and an "out" line with the bits of the results. Prints, for each function, how
many results were compared and how many differ, then the first few that
differ. Exits 1 when any differs or when the files do not hold the same
functions and inputs.
"""
import sys

def read(path):
    r = {}
    with open(path) as f:
        for line in f:
            kind, name, data = line.rstrip("\n").split(" ", 2)
            r[(kind, name)] = data
    return r

def main():
    a, b = read(sys.argv[1]), read(sys.argv[2])
    bad = 0
    if sorted(a) != sorted(b):
        print("the files hold different functions", file=sys.stderr)
        return 1
    total = 0
    for name in sorted({n for k, n in a if k == "out"}):
        ia, ib = a[("in", name)], b[("in", name)]
        oa, ob = a[("out", name)], b[("out", name)]
        if ia != ib:
            print(f"{name}: the inputs differ, so the results cannot be compared", file=sys.stderr)
            bad += 1
            continue
        n = len(oa) // 16
        # width of an argument group in the input line: 16 or 32 hex digits
        w = len(ia) // n
        diff = [i for i in range(n) if oa[i * 16:i * 16 + 16] != ob[i * 16:i * 16 + 16]]
        total += n
        print(f"{name:8} {n} compared, {len(diff)} differ")
        for i in diff[:5]:
            args = " ".join(ia[i * w + j:i * w + j + 16] for j in range(0, w, 16))
            print(f"    input {i}: {args}  first {oa[i*16:i*16+16]}  second {ob[i*16:i*16+16]}")
        bad += len(diff)
    print(f"{total} results compared, {bad} differ")
    return 1 if bad else 0

sys.exit(main())
