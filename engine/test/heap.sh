#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Run the tests of the allocator (heap-test.c) on a build of the native test
# host: the scenarios (limit, budget, chunks, resizes, host refusals, release)
# and random sequences with a seed.
#
#   engine/test/heap.sh [-m MODE] [-s SEEDS] [-n STEPS]
#
# MODE is a mode of build.sh (default plain), and it must be built already:
# engine/test/build.sh MODE. SEEDS (default 5) is how many seeds the random
# sequences run with, STEPS (default 300000) the steps of each. The status is 0
# when everything passed. Wrap the script in taskset, flock or systemd-run to
# keep it to the CPUs and the memory you give it.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
mode=plain
seeds=5
steps=300000
while getopts m:s:n: opt; do
	case $opt in
	m) mode=$OPTARG ;;
	s) seeds=$OPTARG ;;
	n) steps=$OPTARG ;;
	*) echo "usage: heap.sh [-m MODE] [-s SEEDS] [-n STEPS]" >&2; exit 2 ;;
	esac
done
bin=$here/out/$mode/heap-test
[ -x "$bin" ] || { echo "heap.sh: $bin is missing: run engine/test/build.sh $mode" >&2; exit 2; }

# An AddressSanitizer or UndefinedBehaviorSanitizer finding stops the run.
ASAN_OPTIONS=exitcode=86:detect_leaks=1${ASAN_OPTIONS:+:$ASAN_OPTIONS}
UBSAN_OPTIONS=exitcode=86:print_stacktrace=1${UBSAN_OPTIONS:+:$UBSAN_OPTIONS}
export ASAN_OPTIONS UBSAN_OPTIONS

"$bin" scenarios
i=1
while [ "$i" -le "$seeds" ]; do
	"$bin" stress "$i" "$steps"
	i=$((i + 1))
done

echo "heap.sh: all passed"
