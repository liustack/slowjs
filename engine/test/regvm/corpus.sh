#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Run every script the native test host knows and compare each run with its
# golden (rvdiff -golden, see rvdiff.c): the scripts of compare.sh (those of
# engine/test/compare, its recorded and upstream folders and testdata), the
# scripts of engine/test/regvm/corpus and engine/test/regvm/js (the tests of
# each package of the register loop), and any PATH given. Each runs three
# ways: as compare.sh runs it, with -no-fused-ops, and with -strict (as strict
# code, so that the pass converts more of it). Then unit, unit-oom and
# unit-api run and must end as their goldens: the same status and output,
# but for the count of "N checks passed" (the out of memory sweep of
# unit-oom has a step for each allocation, and their number moves with the
# engine).
#
#   engine/test/regvm/corpus.sh [-j N] [-t SECONDS] [-ignore KINDS] [-golden DIR] [-update] [MODE] [PATH...]
#
# The goldens are those of engine/test/regvm/golden/corpus, recorded from the
# old loop while it existed (see golden.sh), or those of -golden DIR
# (DIR/units/UNIT.txt for the unit programs). -update writes the golden of a
# case that differs from the run or has none: read the diff before it goes
# into a commit.
#
# -j is the number of scripts run at once (default 2). -t is the time each run
# may take (default 60 s, 240 s in the sanitizer modes, which run 4 to 8
# times slower): a run the limit ends is not compared (rvdiff prints SLOW).
# MODE is a mode of build.sh of this folder (default plain), built already:
# engine/test/regvm/build.sh MODE. BIN_DIR names another folder of the
# programs (a build with -DRV_STATS, for one). Results are in
# engine/test/out/rvdiff-MODE/ (OUT_DIR changes it): rvdiff.log, the
# transcripts of each script that differs in diffs/, and units.log. It exits
# with 1 when anything differs.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
test=$(dirname "$here")
root=$(dirname "$(dirname "$test")")
jobs=2
ign=
tmo=
golden=$here/golden/corpus
update=
while [ $# -gt 0 ]; do
	case $1 in
	-j) jobs=$2; shift 2 ;;
	-t) tmo=$2; shift 2 ;;
	-ignore) ign=$2; shift 2 ;;
	-golden) golden=$2; shift 2 ;;
	-update) update=1; shift ;;
	*) break ;;
	esac
done
mode=${1:-plain}
[ $# -gt 0 ] && shift
bindir=${BIN_DIR:-$test/out/$mode}
case $mode in
*san*) tmo=${tmo:-240} ;;
*) tmo=${tmo:-60} ;;
esac
out=${OUT_DIR:-$test/out/rvdiff-$mode}
bin=$bindir/rvdiff
[ -x "$bin" ] || { echo "corpus.sh: $bin is missing: run engine/test/regvm/build.sh $mode" >&2; exit 2; }
# a build of the tree now (see engine/test/stamp.sh)
. "$test/stamp.sh"
stamp_check "$(dirname "$test")" "$bindir" || exit 2
[ -d "$golden" ] || [ -n "$update" ] || { echo "corpus.sh: no goldens in $golden (-golden DIR names others, -update writes them)" >&2; exit 2; }
mkdir -p "$golden/units"
rm -rf "$out"
mkdir -p "$out/gen" "$out/diffs"

UBSAN_OPTIONS=${UBSAN_OPTIONS:+$UBSAN_OPTIONS:}halt_on_error=1:exitcode=86:print_stacktrace=1
export UBSAN_OPTIONS

# The two scripts compare.sh makes before it runs them (see there).
r="$test/compare/recorded"
{
	printf 'var shikiRegexps = '
	cat "$r/shiki/regexps.json"
	printf ';\nvar shikiInput = "'
	sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e 's/	/\\t/g' "$r/shiki/input.md" | awk '{ printf "%s\\n", $0 }'
	printf '";\n'
	cat "$r/shikire.js"
} > "$out/gen/shikire.js"
{
	printf '"use strict"; var std = { gc: gc };\n'
	printf 'var os = { setTimeout(f) { Promise.resolve().then(() => {}).then(() => {}).then(f); } };\n'
	cat "$test/compare/upstream/test_builtin.js"
} > "$out/gen/test_builtin.js"

# SCRIPT OPTIONS, as compare.sh runs them
base=$out/base.list
: > "$base"
for f in "$r"/*.js; do
	[ "$(basename "$f")" = shikire.js ] && continue
	echo "$f -raw" >> "$base"
done
echo "$r/regexcache.js -raw -regexp-cache=4,1048576" >> "$base"
echo "$r/regexcache.js -raw -regexp-cache=0,0" >> "$base"
echo "$out/gen/shikire.js -raw" >> "$base"
for f in "$root"/testdata/*.js; do
	echo "$f -raw" >> "$base"
done
echo "$root/testdata/error_stack.js -raw -result" >> "$base"
echo "$root/testdata/error_stack.js -raw -call" >> "$base"
for f in "$test"/compare/upstream/*.js; do
	[ "$(basename "$f")" = test_builtin.js ] && continue
	echo "$f -raw" >> "$base"
done
echo "$out/gen/test_builtin.js -raw" >> "$base"
# the scripts of engine/test/compare name their options themselves
for f in "$test"/compare/*.js "$test"/compare/*.mjs "$here"/corpus/*.js "$here"/js/*.js; do
	[ -f "$f" ] && echo "$f" >> "$base"
done
for p in "$@"; do
	if [ -d "$p" ]; then
		for f in "$p"/*.js; do [ -f "$f" ] && echo "$f" >> "$base"; done
	else
		echo "$p" >> "$base"
	fi
done

cases=$out/cases.list
{
	cat "$base"
	sed 's/$/ -no-fused-ops/' "$base"
	grep -v '\.mjs' "$base" | sed 's/$/ -strict/'
} > "$cases"

# the output of a unit program as compared: its count of checks left out
norm() {
	sed "s/^\(unit[-a-z]*\): [0-9]* checks passed\$/\1: N checks passed/" "$1.raw" > "$1"
}

status=0
"$bin" -q -j "$jobs" -t "$tmo" -o "$out/diffs" ${ign:+-ignore "$ign"} -golden "$golden" -root "$root" ${update:+-update} -cases "$cases" > "$out/rvdiff.log" 2>&1 || status=$?
grep '^DIFF\|^NOTE\|^SLOW\|^NOUPDATE\|^UPDATE' "$out/rvdiff.log" || true
grep "^rvdiff: engine \\|^rvdiff: [0-9]* scripts" "$out/rvdiff.log" || true
[ $status -le 1 ] || { echo "corpus.sh: rvdiff failed with status $status, see $out/rvdiff.log" >&2; exit 1; }

# the unit programs, each once against its golden
: > "$out/units.log"
for u in unit-rv unit-oom-rv unit-api-rv; do
	[ -x "$bindir/$u" ] || continue
	s=0
	g=$golden/units/$u.txt
	"$bindir/$u" > "$out/$u.raw" 2>&1 || s=$?
	norm "$out/$u"
	echo "--- status $s" >> "$out/$u"
	if [ ! -f "$g" ] || ! cmp -s "$g" "$out/$u"; then
		if [ -f "$g" ]; then
			echo "DIFF $u: not as its golden $g, see $out/$u"
		else
			echo "DIFF $u: no golden $g, see $out/$u"
		fi | tee -a "$out/units.log"
		status=1
		if [ -n "$update" ]; then
			cp "$out/$u" "$g"
			echo "UPDATE $g"
		fi
	else
		echo "ok $u (status $s)" >> "$out/units.log"
	fi
done
echo "units: $(grep -c '^ok' "$out/units.log") the same, $(grep -c '^DIFF' "$out/units.log" || true) differ"
exit $status
