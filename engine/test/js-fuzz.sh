#!/bin/sh
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Run js-fuzz, the libFuzzer target of the engine (js-fuzz.c), for a fixed
# time, under AddressSanitizer and UndefinedBehaviorSanitizer. It must be
# built already: engine/test/build.sh fuzz.
#
#   engine/test/js-fuzz.sh [-f SECONDS] [-c CORPUS] [SEEDS...]
#
# SECONDS (default 300) is how long it runs, with the words of js-fuzz.dict
# (-dict). CORPUS (default engine/test/out/js-fuzz-corpus) is the folder it
# keeps the inputs it finds in, and starts from. The first run seeds it with
# the scripts and modules of compare.sh and of the register loop, and with
# the inputs in js-fuzz-seeds, which hold regexps and JSON texts (see
# js-fuzz.c for the form of an input). SEEDS are more folders to start from,
# such as test262 files made into inputs. An input that fails is kept as
# crash-*, timeout-* or oom-* in the current folder. The status is 0 when
# nothing failed.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
secs=300
corpus=$here/out/js-fuzz-corpus
while getopts f:c: opt; do
	case $opt in
	f) secs=$OPTARG ;;
	c) corpus=$OPTARG ;;
	*) echo "usage: js-fuzz.sh [-f SECONDS] [-c CORPUS] [SEEDS...]" >&2; exit 2 ;;
	esac
done
shift $((OPTIND - 1))
bin=$here/out/fuzz/js-fuzz
[ -x "$bin" ] || { echo "js-fuzz.sh: $bin is missing: run engine/test/build.sh fuzz" >&2; exit 2; }

if [ ! -d "$corpus" ]; then
	mkdir -p "$corpus"
	cp "$here"/js-fuzz-seeds/* "$corpus"/
	# the scripts, each as a script or a module (first byte 0 or 1), and
	# every fourth as bytecode too (bit 4 set)
	i=0
	for f in "$here"/compare/*.js "$here"/compare/*.mjs "$here"/regvm/corpus/*.js "$here"/regvm/js/*.js; do
		[ -f "$f" ] || continue
		i=$((i + 1))
		m=0
		case $f in *.mjs) m=1 ;; esac
		{ printf "\\00$m"; cat "$f"; } > "$corpus/s$i"
		[ $((i % 4)) -ne 0 ] || { printf "\\02$m"; cat "$f"; } > "$corpus/b$i"
	done
fi

# A finding stops the target, and libFuzzer keeps the input.
UBSAN_OPTIONS=print_stacktrace=1${UBSAN_OPTIONS:+:$UBSAN_OPTIONS}
# The host asks a runtime to stop after a second or two, on top of the polls
# js-fuzz counts.
HOST_TIMEOUT_SEC=${HOST_TIMEOUT_SEC:-1}
export UBSAN_OPTIONS HOST_TIMEOUT_SEC
"$bin" -dict="$here/js-fuzz.dict" -max_total_time="$secs" -timeout=30 -rss_limit_mb=4096 -max_len=16384 -print_final_stats=1 "$corpus" "$@"
echo "js-fuzz.sh: nothing failed"
