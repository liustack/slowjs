#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# train.sh: train the profile make gen compiles the engine with, into
# engine/pgo/profile.txt (see engine/pgo/README.md). Run it from anywhere
# inside the repository on a machine with LLVM 22.1.8, then make gen and
# make gen ARCH=arm64. It takes about two minutes.
#
# The engine and its libc are built as the generator compiles them, with
# -fprofile-instr-generate (engine/test/build.sh pgo). The profile holds:
#   - the pages of engine/pgo/pages, each rendered twice, with the counters
#     set to zero when the second pass starts: warm renders only, no load of
#     a bundle and no first render;
#   - the micro benchmarks of compare/testdata at the counts of
#     engine/pgo/micro.txt, so that a path the pages never take is not left
#     without a count.
#
# Environment:
#   LLVM_BIN   bin folder of LLVM 22.1.8 (default /usr/lib/llvm-22/bin)
#   TRAIN_OUT  work folder (default .build/pgo-train)
set -eu

cd "$(dirname "$0")/../.."
llvm=${LLVM_BIN:-/usr/lib/llvm-22/bin}
want=22.1.8
got=$("$llvm/clang" --version 2>/dev/null | sed -n 's/.*clang version \([0-9][0-9.]*\).*/\1/p' | head -n 1)
if [ "$got" != "$want" ]; then
	echo "train.sh: clang in $llvm is '${got:-missing}': the profile is trained with LLVM $want only (set LLVM_BIN)" >&2
	exit 1
fi
out=${TRAIN_OUT:-.build/pgo-train}
rm -rf "$out"
mkdir -p "$out/micro"

cc=$llvm/clang
if [ "$(uname -s)" = Darwin ]; then
	cc="$cc -isysroot $(xcrun --show-sdk-path)"
fi
CC=$cc engine/test/build.sh pgo "$out/host" > "$out/build.log"

# render APP PAGE...: the pages of APP twice, the counters set to zero
# before the second pass (reset=K, engine/test/render.c).
p=engine/pgo/pages
render() {
	app=$1
	shift
	ins=
	for pg in "$@" "$@"; do
		ins="$ins,$p/$app/in/$pg.json"
	done
	LLVM_PROFILE_FILE="$out/$app.profraw" "$out/host/render" tifa=1 young=1 prelude="$p/fakeintl.js" \
		bundle="$p/$app/$app.js" inputs="${ins#,}" warm=2 n=8 tag="$app" reset=$# > "$out/$app.txt"
}
render shop wb-studio wb-order-studio wb-stats-studio store-en-US-home store-en-US-products \
	store-en-US-product store-en-US-journal store-en-US-post store-en-US-cart store-en-US-checkout
render mui home

while read -r name n; do
	sed "s/\\\$N/$n/" "compare/testdata/$name.js" > "$out/micro/$name.js"
	LLVM_PROFILE_FILE="$out/micro/$name.profraw" "$out/host/jsrun" "$out/micro/$name.js" > "$out/micro/$name.txt"
done < engine/pgo/micro.txt

"$llvm/llvm-profdata" merge -o "$out/profile.profdata" "$out"/*.profraw "$out"/micro/*.profraw
"$llvm/llvm-profdata" merge --text -o engine/pgo/profile.txt "$out/profile.profdata"
echo "engine/pgo/profile.txt: $(grep -c '^# Func Hash:' engine/pgo/profile.txt) functions"
