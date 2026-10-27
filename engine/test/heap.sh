#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Run the tests of the allocator (heap-test.c) on a build of the native test
# host: the scenarios (limit, budget, chunks, resizes, host refusals, release,
# front cache, cache of large blocks, retry after a failure), what the poison
# covers and that a use after free is reported (poison-test, in asan-heap
# mode), random sequences with a seed on the heap alone and on the runtime, and
# the libFuzzer target for a fixed time.
#
#   engine/test/heap.sh [-m MODE] [-s SEEDS] [-n STEPS] [-f SECONDS]
#
# MODE is a mode of build.sh that has the engine's own allocator: plain
# (default), asan-heap or ubsan, and it must be built already:
# engine/test/build.sh MODE. SEEDS (default 5) is how many seeds the random
# sequences run with, STEPS (default 300000) the steps of each. SECONDS (default
# 0, none) is how long the fuzz target runs: it builds engine/test/out/fuzz
# first, starts from the inputs in engine/test/out/fuzz-corpus and keeps the
# new ones there. The status is 0 when everything passed. Wrap the script in
# taskset, flock or systemd-run to keep it to the CPUs and the memory you give
# it.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
mode=plain
seeds=5
steps=300000
fuzz=0
while getopts m:s:n:f: opt; do
	case $opt in
	m) mode=$OPTARG ;;
	s) seeds=$OPTARG ;;
	n) steps=$OPTARG ;;
	f) fuzz=$OPTARG ;;
	*) echo "usage: heap.sh [-m MODE] [-s SEEDS] [-n STEPS] [-f SECONDS]" >&2; exit 2 ;;
	esac
done
bin=$here/out/$mode/heap-test
[ -x "$bin" ] || { echo "heap.sh: $bin is missing: run engine/test/build.sh $mode" >&2; exit 2; }

# An AddressSanitizer or UndefinedBehaviorSanitizer finding stops the run.
ASAN_OPTIONS=exitcode=86:detect_leaks=1${ASAN_OPTIONS:+:$ASAN_OPTIONS}
UBSAN_OPTIONS=halt_on_error=1:exitcode=86:print_stacktrace=1${UBSAN_OPTIONS:+:$UBSAN_OPTIONS}
export ASAN_OPTIONS UBSAN_OPTIONS

"$bin" scenarios
# asan-heap mode has poison-test too: what the poison covers
[ ! -x "$here/out/$mode/poison-test" ] || "$here/out/$mode/poison-test"
i=1
while [ "$i" -le "$seeds" ]; do
	"$bin" stress "$i" "$steps"
	"$bin" stress-runtime "$i" $((steps * 3))
	i=$((i + 1))
done

if [ "$fuzz" -gt 0 ]; then
	fuzzbin=$here/out/fuzz/heap-fuzz
	[ -x "$fuzzbin" ] || { echo "heap.sh: $fuzzbin is missing: run engine/test/build.sh fuzz" >&2; exit 2; }
	corpus=$here/out/fuzz-corpus
	mkdir -p "$corpus"
	"$fuzzbin" -max_total_time="$fuzz" -timeout=60 -rss_limit_mb=3072 -print_final_stats=1 "$corpus"
fi
echo "heap.sh: all passed"
