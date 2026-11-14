#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# train.sh: train the profile make gen compiles the engine with, into
# engine/pgo/profile.txt (see engine/pgo/README.md). Run it from anywhere
# inside the repository on a machine with LLVM 22.1.8, then make gen and
# make gen ARCH=arm64. It takes about 30 seconds.
#
# The engine and its libc are built as the generator compiles them, with
# -fprofile-instr-generate (engine/test/build.sh pgo). The profile holds the
# pages of engine/pgo/pages, each rendered twice, with the counters set to zero
# when the second pass starts: warm renders only, no load of a bundle and no
# first render.
#
# Environment:
#   LLVM_BIN   bin folder of LLVM 22.1.8 (default: what scripts/llvm-bin.sh finds)
#   TRAIN_OUT  work folder (default .build/pgo-train)
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
	LLVM_PROFILE_FILE="$out/$app.profraw" "$out/host/render" tifa=1 young=1 prelude="$p/fakeintl.js" \
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

"$llvm/llvm-profdata" merge -o "$out/profile.profdata" "$out/shop.profraw" "$out/mui.profraw"
"$llvm/llvm-profdata" merge --text -o engine/pgo/profile.txt "$out/profile.profdata"
echo "engine/pgo/profile.txt: $(grep -c '^# Func Hash:' engine/pgo/profile.txt) functions"
