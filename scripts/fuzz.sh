#!/bin/sh
# Copyright (c) 2026 The liustack/tifajs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE at the
# root of this repository.
#
# fuzz.sh: every Go fuzz target of the root module (each func FuzzX of a
# _test.go file), one after another, each for FUZZTIME. A target that fails
# leaves its input in testdata/fuzz/FuzzX of its package, which go test then
# runs as a test, and the script goes on with the others. It exits 1 when a
# target failed. The nightly run (.github/workflows/nightly.yml) runs it, and
# docs/internals/testing.md says how to run a target for longer.
#
#   scripts/fuzz.sh [PATTERN]
#
# PATTERN, a grep -E pattern, picks the targets whose package and name it
# matches, as in "url FuzzVM" or "FuzzEval|FuzzRegExp".
#
# Environment:
#   FUZZTIME          how long each target runs (default 3m)
#   FUZZ_LIST         1: print the targets and stop
set -eu
cd "$(dirname "$0")/.."

fuzztime=${FUZZTIME:-3m}
pattern=${1:-.}

# "PACKAGE TARGET" lines, the packages as ./dir, of the root module only
targets=$(grep -rl --include='*_test.go' '^func Fuzz' . |
	grep -v -e '^\./compare/' -e '^\./\.build/' -e '/testdata/' |
	while read -r f; do
		d=$(dirname "$f")
		grep -h '^func Fuzz[A-Za-z0-9_]*(f \*testing\.F)' "$f" |
			sed "s|^func \(Fuzz[A-Za-z0-9_]*\)(.*|$d \1|"
	done | sort | grep -E -- "$pattern" || true)
[ -n "$targets" ] || { echo "fuzz.sh: no target matches $pattern" >&2; exit 2; }
if [ "${FUZZ_LIST:-0}" = 1 ]; then
	echo "$targets"
	exit 0
fi

failed=
status=0
echo "$targets" | {
	while read -r pkg name; do
		echo "== $pkg $name ($fuzztime)"
		# -fuzzminimizetime bounds the time a new input is shrunk: an
		# input that runs to the eval timeout would take the whole minute
		# of the default.
		if ! go test -run '^$' -fuzz "^$name\$" -fuzztime "$fuzztime" -fuzzminimizetime 10s "$pkg"; then
			failed="$failed $pkg:$name"
			status=1
		fi
	done
	[ -z "$failed" ] || echo "fuzz.sh: failed:$failed" >&2
	exit $status
}
