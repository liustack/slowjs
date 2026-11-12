#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Sum the lines "fallback ..." of engine/test/regvm/rvfallback.h (from
# jsrun -regvm-why, or from a program of rvhook.h run with RV_REGVM=2 and
# RV_FALLBACK_LOG=FILE) by reason and detail: the functions that ran in the
# old loop, and their runs there, apart for the files under harness/ (the
# harness of test262) and the rest. A function is counted once for each
# runtime it ran in, and once in distinct, by its place in its file.
#
#   engine/test/regvm/fallbacks.sh FILE...
#
# Empty output means that nothing ran in the old loop: the gate for removing
# it.
set -eu
LC_ALL=C
export LC_ALL
cat "$@" | awk -F '\t' '
$1 == "fallback" {
	where = ($5 ~ /(^|\/)harness\//) ? "harness" : "test"
	key = $2 "\t" ($3 == "" ? "-" : $3)
	fn[key] += 1; runs[key] += $6
	fnw[key, where] += 1
	if (!((key, $5) in seen)) { seen[key, $5] = 1; dist[key] += 1; tdist += 1 }
	tfn += 1; truns += $6
}
END {
	if (tfn == 0)
		exit 0
	printf "%-34s %-40s %9s %9s %9s %9s %12s\n", "reason", "detail", "functions", "(test)", "(harness)", "distinct", "runs"
	for (k in fn) {
		split(k, p, "\t")
		printf "%-34s %-40s %9d %9d %9d %9d %12d\n", p[1], p[2], fn[k], fnw[k, "test"], fnw[k, "harness"], dist[k], runs[k] | "sort -k1,1 -s"
	}
	close("sort -k1,1 -s")
	printf "%-34s %-40s %9d %9s %9s %9d %12d\n", "all", "", tfn, "", "", tdist, truns
}'
