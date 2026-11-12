#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Run test262 with the register loop off and on, at the same time, and
# compare: the errors each way (both must be the 33 of
# engine/test/test262_errors.txt), and, with one runner thread each, the
# report of every test (what it threw, which tests failed).
#
#   engine/test/regvm/test262-both.sh [-m MODE] [-T THREADS] [-z ARCHIVE] [-p PATH]...
#
# MODE is a mode of engine/test/regvm/build.sh (default plain), built
# already: it runs its run-test262-rv through engine/test/test262.sh, with
# RV_REGVM=0 and RV_REGVM=1. THREADS is the threads of each run (default 1,
# the only count with a report). ARCHIVE is the test262 archive, as for
# test262.sh. PATH runs only the tests under test262/test/PATH, as
# test262.sh -p does (built-ins/Array, language/statements/for-of): -p again
# adds another. Results are in engine/test/out/test262-rv-MODE-off and -on, and
# the comparison in engine/test/out/test262-rv-MODE.diff. It exits with 1 when
# the two ways differ or either differs from the known errors.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
test=$(dirname "$here")
mode=plain
threads=1
archive=
paths=
while getopts m:T:z:p: opt; do
	case $opt in
	m) mode=$OPTARG ;;
	T) threads=$OPTARG ;;
	z) archive=$OPTARG ;;
	p) paths="$paths -p $OPTARG" ;;
	*) echo "usage: test262-both.sh [-m MODE] [-T THREADS] [-z ARCHIVE] [-p PATH]..." >&2; exit 2 ;;
	esac
done
[ -x "$test/out/$mode/run-test262-rv" ] || { echo "test262-both.sh: run engine/test/regvm/build.sh $mode first" >&2; exit 2; }
# a build of the tree now (see engine/test/stamp.sh)
. "$test/stamp.sh"
stamp_check "$(dirname "$test")" "$test/out/$mode" || exit 2
# test262.sh runs out/MODE/run-test262: a folder of its own whose runner is
# the one with the hook
rvmode=rv-$mode
mkdir -p "$test/out/$rvmode"
ln -sf "../$mode/run-test262-rv" "$test/out/$rvmode/run-test262"

LC_ALL=C
export LC_ALL
off=$test/out/test262-rv-$mode-off
on=$test/out/test262-rv-$mode-on
s0=0
s1=0
RV_REGVM=0 "$test/test262.sh" -m "$rvmode" -T "$threads" -w "$off" ${archive:+-z "$archive"} $paths > "$off.log" 2>&1 &
p0=$!
RV_REGVM=1 "$test/test262.sh" -m "$rvmode" -T "$threads" -w "$on" ${archive:+-z "$archive"} $paths > "$on.log" 2>&1 &
p1=$!
wait $p0 || s0=$?
wait $p1 || s1=$?
echo "off: $(grep '^Result' "$off/run.log" 2>/dev/null || echo 'no result') ($(tail -n 1 "$off.log"))"
echo "on:  $(grep '^Result' "$on/run.log" 2>/dev/null || echo 'no result') ($(tail -n 1 "$on.log"))"

d=$test/out/test262-rv-$mode.diff
status=0
if [ -f "$off/found.sorted" ] && [ -f "$on/found.sorted" ]; then
	if diff "$off/found.sorted" "$on/found.sorted" > "$d"; then
		echo "errors: the same $(wc -l < "$on/found.sorted" | tr -d ' ') both ways"
	else
		echo "errors: differ, see $d"
		cat "$d"
		status=1
	fi
else
	echo "test262-both.sh: a run left no errors, see $off.log and $on.log" >&2
	status=1
fi
# the report of each test: the lines without the times
if [ -f "$off/run/test262_report.txt" ] && [ -f "$on/run/test262_report.txt" ]; then
	for w in "$off" "$on"; do
		grep -v "^ time: [0-9]* ms\$" "$w/run/test262_report.txt" > "$w/report.cmp" || true
	done
	if diff "$off/report.cmp" "$on/report.cmp" > "$d.report"; then
		echo "report: the same, $(wc -l < "$on/report.cmp" | tr -d ' ') lines"
	else
		echo "report: differs in $(grep -c '^[<>]' "$d.report") lines, see $d.report"
		head -40 "$d.report"
		status=1
	fi
fi
[ $s0 -eq 0 ] || { echo "off: test262.sh exited with $s0 (see $off.log)"; status=1; }
[ $s1 -eq 0 ] || { echo "on: test262.sh exited with $s1 (see $on.log)"; status=1; }
exit $status
