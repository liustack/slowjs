#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Compare the math of the C tree, with engine/libc, with the Go engine, in two
# ways. Both use inputs from the fixed generator of mathcmp.js, 100000 for each
# function, and every result has to be the same bit for bit.
#
# - Math.* as JavaScript sees it: mathcmp.js runs on the C tree (mathdump) and
#   on the Go engine (internal/mathdump), for 33 operations. Not every one
#   reaches the libc function of its name: Math.round is the engine's own code,
#   and Uint8ClampedArray stores and Atomics.pause use lrint and modf only for
#   a conversion and a test.
# - The functions of libc themselves: libmath calls the 33 math functions the
#   engine takes from engine/libc (round, modf and lrint included), and
#   internal/libmdump calls the same ones of modernc.org/libc, the musl that
#   ccgo translated and the Go engine runs, on the same inputs.
#
# The Go side is the reference only on linux/amd64: there modernc.org/libc is
# musl's C. Anywhere else the script stops. A NaN is compared by its bits in the
# libc test and as the one NaN of JavaScript in the other. Results are in
# engine/test/out/math-MODE/.
#
#   engine/test/math.sh [MODE]
#
# MODE is a mode of build.sh (default plain). Wrap the script in taskset or
# systemd-run to keep it to the CPUs and the memory you give it.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(dirname "$(dirname "$here")")
mode=${1:-plain}
out=$here/out/math-$mode
js_names="sin cos tan asin acos atan atan2 sinh cosh tanh asinh acosh atanh exp expm1 log log10 log1p log2 pow cbrt hypot floor ceil trunc round sqrt abs min max mod clamp pause"
libc_names="acos acosh asin asinh atan atan2 atanh cbrt ceil cos cosh exp expm1 fabs floor fmax fmin fmod hypot log log10 log1p log2 lrint modf pow round sin sinh sqrt tan tanh trunc"
count=100000

goos=$(cd "$root" && go env GOOS)
goarch=$(cd "$root" && go env GOARCH)
if [ "$(uname -s)" != Linux ] || [ "$(uname -m)" != x86_64 ] || [ "$goos/$goarch" != linux/amd64 ]; then
	echo "math.sh: the reference runs on linux/amd64 only, and this is $(uname -s)/$(uname -m) with GOOS/GOARCH $goos/$goarch" >&2
	exit 2
fi
echo "math.sh: $goos/$goarch, $(cd "$root" && go version | cut -d' ' -f3), source $(git -C "$root" rev-parse --short HEAD 2>/dev/null || echo unknown), $(${CC:-clang} --version | head -1)"

"$here/build.sh" "$mode" > /dev/null
mkdir -p "$out"
(cd "$root" && go build -o "$out/mathdump-go" ./internal/mathdump && go build -o "$out/libmdump-go" ./internal/libmdump)

echo "== Math.* as JavaScript sees it"
# shellcheck disable=SC2086
"$here/out/$mode/mathdump" "$here/mathcmp.js" $js_names > "$out/native.txt"
# shellcheck disable=SC2086
"$out/mathdump-go" "$here/mathcmp.js" $js_names > "$out/go.txt"
python3 "$here/mathcmp.py" --count "$count" --names "$js_names" "$out/native.txt" "$out/go.txt"

echo "== the functions of libc"
"$here/out/$mode/libmath" "$out/native.txt" > "$out/libc-native.txt"
"$out/libmdump-go" "$out/native.txt" > "$out/libc-go.txt"
python3 "$here/mathcmp.py" --count "$count" --names "$libc_names" "$out/libc-native.txt" "$out/libc-go.txt"
