#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Run test262 on the C tree with the native test host.
#
#   engine/test/test262.sh [-m MODE] [-T THREADS] [-w WORKDIR] [-z ARCHIVE]
#
# MODE is a mode of build.sh (default plain), and the host must be built
# already: engine/test/build.sh MODE. THREADS is the number of runner threads
# (default 2). WORKDIR (default engine/test/out/test262-MODE) gets the unpacked
# test262 with test262.diff applied, the configuration and the results:
# run.log, and test262_errors.txt, the errors this run found. ARCHIVE is a copy
# of the pinned test262 archive, which is downloaded when it is not given.
#
# The errors found are compared, line by line, with the 58 known errors in
# engine/test/test262_errors.txt and the script exits 0 only when they are the
# same lines and the runner exited with its own status, 0 or 1. Wrap the script
# in taskset, flock or systemd-run to keep it to the CPUs and the memory you
# give it.
set -eu
LC_ALL=C
export LC_ALL

here=$(cd "$(dirname "$0")" && pwd)
root=$(dirname "$(dirname "$here")")
mode=plain
threads=2
work=
archive=
while getopts m:T:w:z: opt; do
	case $opt in
	m) mode=$OPTARG ;;
	T) threads=$OPTARG ;;
	w) work=$OPTARG ;;
	z) archive=$OPTARG ;;
	*) echo "usage: test262.sh [-m MODE] [-T THREADS] [-w WORKDIR] [-z ARCHIVE]" >&2; exit 2 ;;
	esac
done
[ -n "$work" ] || work=$here/out/test262-$mode
bin=$here/out/$mode/run-test262
[ -x "$bin" ] || { echo "test262.sh: $bin is missing: run engine/test/build.sh $mode" >&2; exit 2; }

# The pinned test262 commit and the SHA-256 of its archive.
zipname=5c8206929d81b2d3d727ca6aac56c18358c8d790.zip
sha256=c34bd60057f1c5128ad5d4e496dc69d712da9a2260eb3308e6f225ba7d3a3096

if command -v sha256sum >/dev/null 2>&1; then sum() { sha256sum "$1" | cut -d' ' -f1; }
else sum() { shasum -a 256 "$1" | cut -d' ' -f1; }; fi

mkdir -p "$work"
work=$(cd "$work" && pwd)
if [ -z "$archive" ]; then
	archive=$here/out/$zipname
	mkdir -p "$here/out"
	[ -f "$archive" ] || curl -fsSL -o "$archive" "https://github.com/tc39/test262/archive/$zipname"
fi
[ "$(sum "$archive")" = "$sha256" ] || { echo "test262.sh: $archive is not the pinned archive (SHA-256)" >&2; exit 2; }

run=$work/run
if [ ! -d "$run/test262" ]; then
	mkdir -p "$run"
	unzip -q "$archive" -d "$run"
	mv "$run/test262-${zipname%.zip}" "$run/test262"
	patch -s -p1 -d "$run/test262" < "$here/test262.diff"
fi
cp "$here/test262.conf" "$run/test262.conf"
: > "$run/test262_errors.txt"

# -u writes the errors found to the error file of the configuration (which
# must exist, and starts empty here) instead of checking them against it, so
# that the lines can be compared exactly. The runner returns 1 when it found
# any error, which is what is expected here.
cd "$run"
# A few tests wait for the clock to move and never end on a fixed one.
HOST_CLOCK=running
export HOST_CLOCK
# A sanitizer that stops the run exits 86, so that it cannot pass for the
# runner's status 1. Options the caller sets come after and win.
ASAN_OPTIONS=exitcode=86${ASAN_OPTIONS:+:$ASAN_OPTIONS}
UBSAN_OPTIONS=exitcode=86${UBSAN_OPTIONS:+:$UBSAN_OPTIONS}
export ASAN_OPTIONS UBSAN_OPTIONS
status=0
"$bin" -v -T "$threads" -c test262.conf -a -u > "$work/run.log" 2>&1 || status=$?
cp "$run/test262_errors.txt" "$work/test262_errors.txt"

grep '^Result: ' "$work/run.log" || { echo "test262.sh: no result line, see $work/run.log (exit status $status)" >&2; exit 1; }
# The runner exits 0 with no error and 1 with errors. Anything else is a crash,
# a signal or a sanitizer, and fails the run even when the errors match.
case $status in
0|1) ;;
*) echo "test262.sh: the runner exited with status $status, see $work/run.log" >&2; exit 1 ;;
esac
sort "$here/test262_errors.txt" > "$work/known.sorted"
sort "$work/test262_errors.txt" > "$work/found.sorted"
if diff "$work/known.sorted" "$work/found.sorted" > "$work/errors.diff"; then
	echo "errors: the $(wc -l < "$work/found.sorted" | tr -d ' ') errors found are the known errors of engine/test/test262_errors.txt"
else
	echo "errors: differ from engine/test/test262_errors.txt, see $work/errors.diff" >&2
	cat "$work/errors.diff" >&2
	exit 1
fi
