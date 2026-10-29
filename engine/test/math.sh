#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Check the math of the C tree, with engine/libc, against the results kept in
# engine/test/math/, bit for bit, in two ways. Both use inputs from the fixed
# generator of mathcmp.js, 100000 for each function.
#
# - Math.* as JavaScript sees it: mathcmp.js runs on the C tree (mathdump), for
#   33 operations. Not every one reaches the libc function of its name:
#   Math.round is the engine's own code, and Uint8ClampedArray stores and
#   Atomics.pause use lrint and modf only for a conversion and a test.
#   Kept in math/js.hashes.
# - The functions of libc themselves: libmath calls the 33 math functions the
#   engine takes from engine/libc (round, modf and lrint included) on the same
#   inputs. Kept in math/libc.hashes.
#
# The files hold, for each function, the line of the inputs and the line of the
# results as "KIND NAME N SHA-256", the SHA-256 being that of the hex digits of
# the line. They were first recorded on linux/amd64 from the previous engine,
# and the C tree gives the same bits on that machine, so they are what musl
# gives and what Math.* gives with it. A NaN is written as the one NaN of JavaScript in
# the first, so that file holds on every machine. In the second it is written
# with its bits, and the sign of the NaN that an operation makes is the
# machine's (x86-64 makes a negative one, arm64 a positive one), so that file is
# checked on x86-64 only and the check says so elsewhere. A line that differs is
# named, with the function. Results are in engine/test/out/math-MODE/
#
#   engine/test/math.sh [MODE]
#
# MODE is a mode of build.sh (default plain). MATH_UPDATE=1 writes the two files
# from what the C tree prints now: read the difference before it goes into a
# commit, as a file kept is a claim about what the engine must print. Wrap the
# script in taskset or systemd-run to keep it to the CPUs and the memory you
# give it.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
mode=${1:-plain}
out=$here/out/math-$mode
js_names="sin cos tan asin acos atan atan2 sinh cosh tanh asinh acosh atanh exp expm1 log log10 log1p log2 pow cbrt hypot floor ceil trunc round sqrt abs min max mod clamp pause"
libc_names="acos acosh asin asinh atan atan2 atanh cbrt ceil cos cosh exp expm1 fabs floor fmax fmin fmod hypot log log10 log1p log2 lrint modf pow round sin sinh sqrt tan tanh trunc"

if command -v sha256sum >/dev/null 2>&1; then sum() { sha256sum | cut -d' ' -f1; }
else sum() { shasum -a 256 | cut -d' ' -f1; }; fi

# hashes FILE: each line "KIND NAME N DATA" as "KIND NAME N SHA-256 of DATA".
hashes() {
	while IFS=' ' read -r kind name n data; do
		printf '%s %s %s %s\n' "$kind" "$name" "$n" "$(printf %s "$data" | sum)"
	done < "$1"
}

mkdir -p "$out"
"$here/build.sh" "$mode" > "$out/build.log" 2>&1 || { cat "$out/build.log" >&2; exit 1; }

echo "== Math.* as JavaScript sees it"
# shellcheck disable=SC2086
"$here/out/$mode/mathdump" "$here/mathcmp.js" $js_names > "$out/native.txt"
hashes "$out/native.txt" > "$out/js.hashes"

echo "== the functions of libc"
"$here/out/$mode/libmath" "$out/native.txt" > "$out/libc-native.txt"
hashes "$out/libc-native.txt" > "$out/libc.hashes"
x86=1
[ "$(uname -m)" = x86_64 ] || x86=0

if [ "${MATH_UPDATE:-}" = 1 ]; then
	[ "$x86" = 1 ] || { echo "math.sh: MATH_UPDATE writes the files on x86-64 only" >&2; exit 2; }
	mkdir -p "$here/math"
	cp "$out/js.hashes" "$here/math/js.hashes"
	cp "$out/libc.hashes" "$here/math/libc.hashes"
	echo "math: wrote $here/math/js.hashes and libc.hashes from the C tree"
	exit 0
fi

failed=0
for k in js libc; do
	if [ "$k" = libc ] && [ "$x86" = 0 ]; then
		echo "math: libc: not checked here, the kept bits are those of x86-64 (the sign of a NaN)"
		continue
	fi
	if diff "$here/math/$k.hashes" "$out/$k.hashes" > "$out/$k.diff"; then
		echo "math: $k: $(wc -l < "$out/$k.hashes" | tr -d ' ') lines are the kept ones"
	else
		echo "math: $k differs from math/$k.hashes in:" >&2
		sed -n 's/^> \([a-z]*\) \([a-z0-9]*\) .*/  \1 \2/p' "$out/$k.diff" >&2
		failed=1
	fi
done
[ "$failed" = 0 ] || exit 1
echo "math: the C tree gives the kept bits"
