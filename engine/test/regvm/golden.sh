#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# The register loop against the goldens of the old loop: what each test gave
# with the old loop, recorded at the last commit that had it (deb70815,
# before the deletion of faa571ce), where the two loops agreed.
#
#   engine/test/regvm/golden.sh [-update] [-inject] [-j N] [-dir G] [MODE]
#
# It checks: corpus.sh -golden G/corpus (every case of the corpus and the
# unit programs), rvgen check of G/seeds/MODE.txt (1,000 fixed seeds of each
# mode of the generator, frozen in internal/rvgen/v1) and, with -inject,
# inject.sh -max 200 -golden G/inject on engine/test/regvm/js and corpus (the
# places of the throws injected there, the old loop's run with each).
# -update writes a golden that differs from the run, or is missing, again
# from the run (corpus, units and, with -inject, the places of a script):
# read the diff before it goes into a commit.
#
# A case with no golden is a difference, so that a script added later
# cannot pass with nothing to be compared with. Its golden comes from Node
# 24 (rvgen node, or the output of the script under Node written by hand)
# or from -update once its output is read and found right. Then it says how
# many cases have none.
#
# G is engine/test/regvm/golden by default. MODE is a mode of build.sh
# (default plain), built already: engine/test/regvm/build.sh MODE. -inject
# also needs engine/test/out/inject-MODE, built with EXTRA_DEFS=-DRV_INJECT
# (see inject.sh). RVGEN names a built rvgen for a machine with no Go. It
# exits with 1 when anything differs.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
test=$(dirname "$here")
root=$(dirname "$(dirname "$test")")
g=$here/golden
update=
inject=
jobs=2
while [ $# -gt 0 ]; do
	case $1 in
	-update) update=1; shift ;;
	-inject) inject=1; shift ;;
	-j) jobs=$2; shift 2 ;;
	-dir) g=$2; shift 2 ;;
	-*) echo "usage: golden.sh [-update] [-inject] [-j N] [-dir G] [MODE]" >&2; exit 2 ;;
	*) break ;;
	esac
done
mode=${1:-plain}
bin=$test/out/$mode/rvdiff
[ -x "$bin" ] || { echo "golden.sh: $bin is missing: run engine/test/regvm/build.sh $mode" >&2; exit 2; }
# a build of the tree now (see engine/test/stamp.sh): inject.sh checks its own
. "$test/stamp.sh"
stamp_check "$(dirname "$test")" "$test/out/$mode" || exit 2
[ -d "$g" ] || { echo "golden.sh: no goldens in $g" >&2; exit 2; }
g=$(cd "$g" && pwd)
# a built rvgen when RVGEN names one
rvgen=${RVGEN:-$test/out/rvgen}
[ -n "${RVGEN:-}" ] || (cd "$root" && go build -o "$rvgen" ./internal/rvgen/cmd/rvgen)

# the output of every check, live and in $log, which is read below for the
# cases that have no golden
log=$test/out/golden-$mode.log
: > "$log"
status=0
check() {
	{ "$@" 2>&1 && echo 0 > "$log.rc" || echo $? > "$log.rc"; } | tee -a "$log"
	[ "$(cat "$log.rc")" = 0 ] || status=1
	rm -f "$log.rc"
}
check "$here/corpus.sh" -j "$jobs" -golden "$g/corpus" ${update:+-update} "$mode"
for m in core full wide; do
	check "$rvgen" check -rvdiff "$bin" -j "$jobs" "$g/seeds/$m.txt"
done
if [ -n "$inject" ]; then
	check "$here/inject.sh" -j "$jobs" -max 200 -golden "$g/inject" ${update:+-update} "$test/out/inject-$mode" "$here/js" "$here/corpus"
fi
missing=$(grep -c ": no golden" "$log" || true)
if [ "$missing" -gt 0 ]; then
	echo "golden.sh: $missing cases have no golden (see $log): each takes its golden from Node 24 or from -update once its output is read" >&2
	status=1
fi
exit $status
