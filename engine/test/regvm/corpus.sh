#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Run every script the native test host knows with the register loop off and
# on, and compare (rvdiff, see rvdiff.c): the scripts of compare.sh (those of
# engine/test/compare, its recorded and upstream folders and testdata), the
# scripts of engine/test/regvm/corpus and engine/test/regvm/js (the tests of
# each package of the register loop), and any PATH given. Each runs three
# ways: as compare.sh runs it, with -no-fused-ops, and with -strict (as strict
# code, so that the pass converts more of it). Then unit, unit-oom and
# unit-api run with the register loop off and on, and must end the same:
# the same status and output, but for the count of "N checks passed" (the
# out of memory sweep of unit-oom has a step for each allocation, and the
# register loop makes more of them).
#
#   engine/test/regvm/corpus.sh [-j N] [-t SECONDS] [-ignore KINDS] [-record DIR | -golden DIR [-update]] [MODE] [PATH...]
#
# -record DIR writes the goldens of the old loop to DIR (rvdiff -record, and
# DIR/units/UNIT.txt: the output of each unit program with the loop off and
# its status) for every case where the two loops agree. -golden DIR runs each
# case and unit once, with the register loop on, against DIR (rvdiff
# -golden), for when there is one loop only. -update writes the golden of a
# case that differs from the run: read the diff before it goes into a commit.
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
record=
golden=
update=
while [ $# -gt 0 ]; do
	case $1 in
	-j) jobs=$2; shift 2 ;;
	-t) tmo=$2; shift 2 ;;
	-ignore) ign=$2; shift 2 ;;
	-record) record=$2; shift 2 ;;
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
"$bin" -q -j "$jobs" -t "$tmo" -o "$out/diffs" ${ign:+-ignore "$ign"} ${record:+-record "$record" -root "$root"} ${golden:+-golden "$golden" -root "$root"} ${update:+-update} -cases "$cases" > "$out/rvdiff.log" 2>&1 || status=$?
grep '^DIFF\|^NOTE\|^SLOW\|^NORECORD\|^UPDATE' "$out/rvdiff.log" || true
grep "^rvdiff: engine \\|^rvdiff: [0-9]* scripts" "$out/rvdiff.log" || true
[ $status -le 1 ] || { echo "corpus.sh: rvdiff failed with status $status, see $out/rvdiff.log" >&2; exit 1; }

# the unit programs: each way, or with -golden once against the goldens
: > "$out/units.log"
for u in unit-rv unit-oom-rv unit-api-rv; do
	[ -x "$bindir/$u" ] || continue
	s0=0
	s1=0
	if [ -n "$golden" ]; then
		g=$golden/units/$u.txt
		RV_REGVM=1 "$bindir/$u" > "$out/$u.on.raw" 2>&1 || s1=$?
		norm "$out/$u.on"
		echo "--- status $s1" >> "$out/$u.on"
		if [ ! -f "$g" ] || ! cmp -s "$g" "$out/$u.on"; then
			echo "DIFF $u: not as its golden $g, see $out/$u.on" | tee -a "$out/units.log"
			status=1
			if [ -n "$update" ]; then
				cp "$out/$u.on" "$g"
				echo "UPDATE $g"
			fi
		else
			echo "ok $u (status $s1)" >> "$out/units.log"
		fi
		continue
	fi
	RV_REGVM=0 "$bindir/$u" > "$out/$u.off.raw" 2>&1 || s0=$?
	RV_REGVM=1 "$bindir/$u" > "$out/$u.on.raw" 2>&1 || s1=$?
	norm "$out/$u.off"
	norm "$out/$u.on"
	if [ $s0 -ne $s1 ] || ! cmp -s "$out/$u.off" "$out/$u.on"; then
		echo "DIFF $u: status off $s0, on $s1, see $out/$u.off and $out/$u.on" | tee -a "$out/units.log"
		status=1
		[ -n "$record" ] && echo "NORECORD units/$u.txt: the loops differ"
	else
		echo "ok $u (status $s0)" >> "$out/units.log"
		if [ -n "$record" ]; then
			mkdir -p "$record/units"
			{ cat "$out/$u.off"; echo "--- status $s0"; } > "$record/units/$u.txt"
		fi
	fi
done
echo "units: $(grep -c '^ok' "$out/units.log") the same, $(grep -c '^DIFF' "$out/units.log" || true) differ"
exit $status
