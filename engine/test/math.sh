#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Compare the Math functions of the C tree, with engine/libc, with the Go
# engine. Both run engine/test/mathcmp.js: 100000 inputs for each function the
# engine takes from a libm, made by a fixed generator, and every result has to
# be the same bit for bit. The Go engine is the reference: on linux/amd64 it
# runs musl's libm as modernc.org/libc translated it.
#
#   engine/test/math.sh [MODE]
#
# MODE is a mode of build.sh (default plain). Results are in
# engine/test/out/math-MODE/ (native.txt, go.txt, and the Go program). Wrap the script in taskset or systemd-run to keep
# it to the CPUs and the memory you give it.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(dirname "$(dirname "$here")")
mode=${1:-plain}
out=$here/out/math-$mode
names="sin cos tan asin acos atan atan2 sinh cosh tanh asinh acosh atanh exp expm1 log log10 log1p log2 pow cbrt hypot floor ceil trunc round sqrt abs min max mod clamp pause"

"$here/build.sh" "$mode" > /dev/null
mkdir -p "$out"
# shellcheck disable=SC2086
"$here/out/$mode/mathdump" "$here/mathcmp.js" $names > "$out/native.txt"
(cd "$root" && go build -o "$out/mathdump-go" ./internal/mathdump)
# shellcheck disable=SC2086
"$out/mathdump-go" "$here/mathcmp.js" $names > "$out/go.txt"
python3 "$here/mathcmp.py" "$out/native.txt" "$out/go.txt"
