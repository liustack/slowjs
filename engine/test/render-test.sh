#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Test the host of engine/test/render.c on the bundles of
# engine/test/render-test, which give what they check as the HTML of their
# render: __tifa_murmur2__ of tifa=1 hashes as @emotion/hash and as tifa's Go
# hash do (murmur2.js), and murmur=0 leaves it out, and gc_renders counts the
# timed renders in which a collection ran, none with young=1 (cycles.js).
#
#   engine/test/render-test.sh [MODE]
#
# MODE is a mode of build.sh (default plain), built already.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
mode=${1:-plain}
bin=$here/out/$mode/render
[ -x "$bin" ] || { echo "render-test.sh: $bin is missing: run engine/test/build.sh $mode" >&2; exit 2; }
out=$here/out/render-test-$mode
rm -rf "$out"
mkdir -p "$out"
printf '{}\n' > "$out/in.json"
bad=0

# check NAME WANT ARGS...: one render of a bundle, its HTML against WANT
check() {
	name=$1 want=$2
	shift 2
	if ! "$bin" inputs="$out/in.json" warm=0 n=0 html="$out" tag="$name" "$@" > "$out/$name.json" 2> "$out/$name.err"; then
		echo "render-test: $name: render failed, see $out/$name.err"; bad=1; return
	fi
	got=$(cat "$out/$name-in.json.html")
	if [ "$got" = "$want" ]; then echo "render-test: $name ok"
	else printf 'render-test: %s: got\n%s\nwant\n%s\n' "$name" "$got" "$want"; bad=1; fi
}

check murmur2 '2011 0
[null,null,null,null,null,null,null,null,true,null,null,true,false]' \
	tifa=1 bundle="$here/render-test/murmur2.js"
check murmur2-off 'no __tifa_murmur2__' tifa=1 murmur=0 bundle="$here/render-test/murmur2.js"
check murmur2-notifa 'no __tifa_murmur2__' bundle="$here/render-test/murmur2.js"
# the collections of the timed renders: the pairs of cycles.js fill the room
# under the threshold every few renders, and a young collection after each
# render frees them before
for y in 0 1; do
	"$bin" bundle="$here/render-test/cycles.js" inputs="$out/in.json" warm=0 n=50 young=$y > "$out/cycles-$y.json" 2> "$out/cycles-$y.err" || { echo "render-test: cycles young=$y: render failed"; bad=1; }
done
gc0=$(sed -n 's/.*"gc_renders":\([0-9]*\).*/\1/p' "$out/cycles-0.json")
gc1=$(sed -n 's/.*"gc_renders":\([0-9]*\).*/\1/p' "$out/cycles-1.json")
if [ "${gc0:-0}" -gt 0 ] && [ "${gc1:-1}" = 0 ]; then echo "render-test: cycles ok ($gc0 of 50 renders collect, none with young=1)"
else echo "render-test: cycles: renders that collect: ${gc0:-?} with young=0 (want some), ${gc1:-?} with young=1 (want 0)"; bad=1; fi
exit $bad
