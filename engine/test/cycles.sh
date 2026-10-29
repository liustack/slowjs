#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Check that every recursion of the engine is bounded by a counted depth. It
# compiles the engine files to LLVM IR as the generator does (freestanding,
# the generator's flags) but at -O0, so that every function of the source is
# there and each count is a call, with -fsanitize=kcfi for the types of
# indirect calls, links them into one module and lists the cycles of its call
# graph that no count cuts (internal/goasm/cmd/cycles). The list must be the
# one in cycles.txt, line for line, which is empty: every recursion goes through a
# count (see docs/fork-changes/c-engine.md). A cycle that appears is a
# recursion with no bound the generator can prove: cut it, or count it.
#
#   engine/test/cycles.sh [LLVM_BIN]
#
# LLVM_BIN is the folder of clang and llvm-link, /usr/lib/llvm-22/bin by
# default as for the generator. The module and the list are in
# engine/test/out/cycles/.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
engine=$(dirname "$here")
root=$(dirname "$engine")
llvm=${1:-/usr/lib/llvm-22/bin}
out=$here/out/cycles
rm -rf "$out"
mkdir -p "$out"

. "$here/flags.sh"
# The flags of the generator's EngineCFlags and freeFlags that change the IR,
# at -O0.
flags="--target=x86_64-linux-gnu -O0 -fno-builtin -fno-omit-frame-pointer -mno-red-zone -fno-optimize-sibling-calls -fcf-protection=none -mstack-alignment=8 $engine_flags -fno-asynchronous-unwind-tables -fno-unwind-tables -fsanitize=kcfi"
lls=
for f in quickjs libregexp libunicode cutils dtoa; do
	"$llvm/clang" $flags -I"$engine/libc" -I"$engine" -DCONFIG_VERSION=\"$engine_version\" \
		-S -emit-llvm -o "$out/$f.ll" "$engine/$f.c"
	lls="$lls $out/$f.ll"
done
"$llvm/llvm-link" -S -o "$out/engine.ll" $lls
(cd "$root" && go run ./internal/goasm/cmd/cycles "$out/engine.ll") > "$out/cycles.txt"
if diff "$here/cycles.txt" "$out/cycles.txt"; then
	echo "cycles: the cycles are the known ones ($(wc -l < "$out/cycles.txt"))"
else
	echo "cycles: DIFFER (< known, > found)"
	exit 1
fi
