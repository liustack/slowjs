#!/bin/sh
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# train.sh: train the profile make gen compiles the engine with, into
# engine/pgo/profile.txt (see engine/pgo/README.md). Run it from anywhere
# inside the repository on a machine with LLVM 22.1.8, then make gen and
# make gen ARCH=arm64. It takes about a minute.
#
# The engine and its libc are built as the generator compiles them, with
# -fprofile-instr-generate (engine/test/build.sh pgo). The profile holds two
# inputs, merged as they are, neither one scaled (scaling a profile is not
# neutral, see docs/changes/generator.md):
#
#   - the pages of engine/pgo/pages, each rendered twice, with the counters set
#     to zero when the second pass starts: warm renders only, no load of a
#     bundle and no first render,
#   - engine/pgo/loops.js, plain loops over the paths the pages never run, so
#     that the compiler does not take them for paths that never run.
#
# Then it checks loops.js (cover.awk): it runs the micro benchmarks of
# compare/testdata, for the check only, and stops when loops.js runs less
# than COVER_FLOOR percent of the paths the pages leave cold and the micro
# benchmarks run, or when a part of loops.js no longer reaches its function.
#
# Environment:
#   LLVM_BIN   bin folder of LLVM 22.1.8 (default: what scripts/llvm-bin.sh finds)
#   TRAIN_OUT  work folder (default .build/pgo-train)
#   COVER_FLOOR  the least percent of the paths loops.js must run (default 95)
set -eu

cd "$(dirname "$0")/../.."
# llvm-bin.sh finds LLVM 22.1.8, checks its version and says what to install.
llvm=$(scripts/llvm-bin.sh)
out=${TRAIN_OUT:-.build/pgo-train}
rm -rf "$out"
mkdir -p "$out"

cc=$llvm/clang
if [ "$(uname -s)" = Darwin ]; then
	cc="$cc -isysroot $(xcrun --show-sdk-path)"
fi
CC=$cc engine/test/build.sh pgo "$out/host" > "$out/build.log"

# render APP PAGE...: the pages of APP twice, the counters set to zero
# before the second pass (reset=K, engine/test/render.c). render stops with an
# error when a page throws. A page that catches its error renders it as JSON
# rather than HTML, so the HTML of each page must start with "<". Each page
# must be in the report and the counters must be written.
p=engine/pgo/pages
render() {
	app=$1
	shift
	mkdir -p "$out/html"
	ins=
	for pg in "$@" "$@"; do
		ins="$ins,$p/$app/in/$pg.json"
	done
	LLVM_PROFILE_FILE="$out/$app.profraw" "$out/host/render" tifa=1 young=1 prelude="$p/fakeintl.js,$p/fakeurl.js" \
		bundle="$p/$app/$app.js" inputs="${ins#,}" warm=2 n=8 tag="$app" html="$out/html" reset=$# > "$out/$app.txt"
	got=$(grep -o '"input":"[^"]*"' "$out/$app.txt" | wc -l | tr -d ' ')
	if [ "$got" != $((2 * $#)) ]; then
		echo "train.sh: $app rendered $got pages, want $((2 * $#)) ($out/$app.txt)" >&2
		exit 1
	fi
	for pg in "$@"; do
		f=$out/html/$app-$pg.json.html
		if [ "$(head -c 1 "$f" 2>/dev/null)" != "<" ]; then
			echo "train.sh: $app did not render $pg: $(head -c 200 "$f" 2>/dev/null)" >&2
			exit 1
		fi
	done
	if [ ! -s "$out/$app.profraw" ]; then
		echo "train.sh: $app wrote no counters ($out/$app.profraw)" >&2
		exit 1
	fi
}
render shop wb-studio wb-order-studio wb-stats-studio store-en-US-home store-en-US-products \
	store-en-US-product store-en-US-journal store-en-US-post store-en-US-cart store-en-US-checkout
render mui home

# run NAME FILE...: the scripts in one process of the native jsrun, counted
# into NAME.profraw. jsrun stops with an error when a script throws.
run() {
	name=$1
	shift
	LLVM_PROFILE_FILE="$out/$name.profraw" "$out/host/jsrun" -raw "$@" > "$out/$name.txt" || {
		echo "train.sh: $name: jsrun failed ($out/$name.txt)" >&2
		exit 1
	}
	if [ ! -s "$out/$name.profraw" ]; then
		echo "train.sh: $name wrote no counters ($out/$name.profraw)" >&2
		exit 1
	fi
}
run loops engine/pgo/loops.js

# The check of loops.js. The micro benchmarks run with N=100, so a path they
# run 100 times runs about once in each turn of a benchmark's loop.
# factorial10 makes a million calls in each turn and runs one. Each part of
# loops.js names one function it is there for (cover.awk).
mkdir -p "$out/micro"
for f in compare/testdata/*.js; do
	n=100
	[ "${f##*/}" = factorial10.js ] && n=1
	sed "s/\\\$N/$n/" "$f" > "$out/micro/${f##*/}"
done
run micro "$out"/micro/*.js
"$llvm/llvm-profdata" merge -o "$out/pages.profdata" "$out/shop.profraw" "$out/mui.profraw"
"$llvm/llvm-profdata" merge --text -o "$out/pages.profile.txt" "$out/pages.profdata"
for p in micro loops; do
	"$llvm/llvm-profdata" merge --text -o "$out/$p.profile.txt" "$out/$p.profraw"
done
# the cold count of the pages: below it, LLVM takes a count for a path that
# does not run (the count of the 99.9999th percentile)
cold=$("$llvm/llvm-profdata" show -detailed-summary "$out/pages.profdata" | awk '/ account for 99\.9999% / { print $7 }')
# the function each part of loops.js is there for, in the order of the parts
need="rv_ta_get js_set_fast_array_element_out_of_line js_dataview_getValue js_dataview_setValue \
lre_exec_backtrack lre_literal js_bigint_cmp js_Date_parse string_get_tzabbr date_fields_find js_number_toPrecision \
js_number_toExponential upper_ascii delete_property set_array_length js_binary_logic_slow \
map_delete_record rv_getel_index rv_instof rv_callma"
awk -v cold="$cold" -v hot=100 -v floor="${COVER_FLOOR:-95}" -v need="$need" -f engine/pgo/cover.awk \
	"$out/pages.profile.txt" "$out/micro.profile.txt" "$out/loops.profile.txt"

"$llvm/llvm-profdata" merge -o "$out/profile.profdata" "$out/pages.profdata" "$out/loops.profraw"
"$llvm/llvm-profdata" merge --text -o engine/pgo/profile.txt "$out/profile.profdata"
echo "engine/pgo/profile.txt: $(grep -c '^# Func Hash:' engine/pgo/profile.txt) functions"
