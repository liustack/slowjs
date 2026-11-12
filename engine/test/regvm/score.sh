#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# The coverage of the register loop on real renders: for each page, the share
# of dispatches that run in each loop and the calls between them, a render,
# and for what runs in the old loop, why, ranked by the dispatches: the
# functions the pass refuses, by the instruction it does not take or the
# reason for the whole function, the functions never converted, and those
# converted but called by a path that does not enter the register loop (see
# rvscore-engine.c). It also checks that each page gives the same HTML with
# the register loop off and on.
#
#   engine/test/regvm/score.sh [-n RENDERS] [-w WARM] [OUTDIR]
#
# Pages: the React page of compare/engines (react/react-ssr.js), and, when
# BENCH_DIR names the folder of tifa's bench bundle (bundles/bench.js,
# prelude/iterator-shim.js, in/bench/table.json and spiral.json), its table
# and spiral pages with tifa's four functions, and when APPS_DIR names the
# folder of the shop and mui apps (prelude/fakeintl.js, APP/bundles/APP.js,
# APP/in/*.json), every page of both apps in one VM each. It builds what it needs into
# engine/test/out/score (with engine/libc from engine/test/out/plain, which it
# builds again when it is missing or of other sources than the tree). OUTDIR (default engine/test/out/score) gets
# score.txt, the whole board (its first line "engine src HASH tools HASH"
# names the sources it was built from), and the files of each run. It exits with 1
# when a page differs between the two loops. It also builds rvwhy (rvwhy.c), the
# same board for one script: SCORE_BUILD_ONLY=1 builds and stops.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
test=$(dirname "$here")
engine=$(dirname "$test")
root=$(dirname "$engine")
n=3
warm=5
while getopts n:w: opt; do
	case $opt in
	n) n=$OPTARG ;;
	w) warm=$OPTARG ;;
	*) echo "usage: score.sh [-n RENDERS] [-w WARM] [OUTDIR]" >&2; exit 2 ;;
	esac
done
shift $((OPTIND - 1))
out=${1:-$test/out/score}
cc=${CC:-clang}
plain=$test/out/plain
bin=$test/out/score
mkdir -p "$out" "$bin"
out=$(cd "$out" && pwd)

# the sources as they are when the build starts, for the board, and the
# libc of the plain build again when it is of other sources (see
# engine/test/stamp.sh)
. "$test/stamp.sh"
built="src $(stamp_src "$engine") tools $(stamp_tools "$engine")"
if ! (EXTRA_DEFS=; stamp_engine_fresh "$engine" "$plain" plain "$cc"); then
	EXTRA_DEFS= "$test/build.sh" plain > "$bin/build-plain.log" 2>&1
fi

. "$test/flags.sh"
common="-O2 -g -Wall -Wno-array-bounds"
free="$engine_flags $(engine_target_flags native) -DJS_HOST_POLL -DRV_SELF_CHECK"
defs="-DJS_REGVM_COUNT -DRV_STATS"
# shellcheck disable=SC2086
$cc $common $free $defs -Wno-unused-function -Wno-sometimes-uninitialized -Wno-unused-result -DCONFIG_PRINT_VALUE -I"$engine/libc" -I"$engine" -DCONFIG_VERSION=\"$engine_version\" \
	-c "$here/rvscore-engine.c" -o "$bin/rvscore-engine.o"
# shellcheck disable=SC2086
$cc $common -fwrapv -funsigned-char -D_GNU_SOURCE -DCONFIG_VERSION=\"$engine_version\" -DCONFIG_PRINT_VALUE \
	-I"$engine" -I"$test" $defs -DJS_RegvmDump=rvscore_dump -c "$test/render.c" -o "$bin/render.o"
$cc $common -c "$here/rvscore-host.c" -o "$bin/rvscore-host.o"
# shellcheck disable=SC2086
# the other engine files and the host, built again each time so that the
# board is of the tree as it is (engine/libc comes from the plain build)
for f in libregexp libunicode cutils dtoa heap tlsf; do
	# shellcheck disable=SC2086
	$cc $common $free -DCONFIG_PRINT_VALUE -I"$engine/libc" -I"$engine" -DCONFIG_VERSION=\"$engine_version\" -c "$engine/$f.c" -o "$bin/$f.o"
done
$cc $common -fwrapv -funsigned-char -D_GNU_SOURCE -DCONFIG_VERSION=\"$engine_version\" -DCONFIG_PRINT_VALUE -I"$engine" -I"$test" -c "$test/host.c" -o "$bin/host.o"
$cc "$bin/rvscore-engine.o" "$bin/libregexp.o" "$bin/libunicode.o" "$bin/cutils.o" "$bin/dtoa.o" \
	"$bin/heap.o" "$bin/tlsf.o" "$plain"/libc/*.o "$bin/host.o" "$bin/rvscore-host.o" "$bin/render.o" -o "$bin/render" -lpthread
# rvwhy: the same board for one script
# shellcheck disable=SC2086
$cc $common -Wno-unused-function -fwrapv -funsigned-char -D_GNU_SOURCE -DCONFIG_VERSION=\"$engine_version\" -DCONFIG_PRINT_VALUE \
	-I"$engine" -I"$test" -c "$here/rvwhy.c" -o "$bin/rvwhy.o"
$cc "$bin/rvscore-engine.o" "$bin/libregexp.o" "$bin/libunicode.o" "$bin/cutils.o" "$bin/dtoa.o" \
	"$bin/heap.o" "$bin/tlsf.o" "$plain"/libc/*.o "$bin/host.o" "$bin/rvscore-host.o" "$bin/rvwhy.o" -o "$bin/rvwhy" -lpthread
[ "${SCORE_BUILD_ONLY:-}" = 1 ] && exit 0

# the React page as render.c wants a bundle: startRender(json) leaves the
# HTML in renderResult
printf 'var startRender = function (json) { globalThis.renderResult = render(); };\n' > "$out/react-start.js"
printf '{}\n' > "$out/react.json"

board=$out/score.txt
echo "engine $built" > "$board"
status=0

# page NAME BUNDLE INPUT PRELUDE TIFA
page() {
	name=$1
	for m in 0 1; do
		mkdir -p "$out/$name-$m"
		RV_STATS=1 RV_OUT="$out/$name-$m/score.txt" RVSCORE_RENDERS=$n \
			"$bin/render" bundle="$2" inputs="$3" prelude="$4" tifa="$5" warm="$warm" n="$n" \
			regvm=$m html="$out/$name-$m" tag="$name" > "$out/$name-$m/render.json" 2> "$out/$name-$m/render.err" || {
			echo "score.sh: $name with the register loop $m failed, see $out/$name-$m/render.err" >&2
			exit 1
		}
	done
	{
		echo "== $name"
		# every page of the render (an app gives one HTML file a page)
		same=0 differ=0 bytes=0
		for f in "$out/$name-0/"*.html; do
			g="$out/$name-1/$(basename "$f")"
			if cmp -s "$f" "$g"; then
				same=$((same + 1))
				bytes=$((bytes + $(wc -c < "$f")))
			else
				differ=$((differ + 1))
				echo "HTML: $(basename "$f") DIFFERS between the loops"
			fi
		done
		if [ $differ -eq 0 ] && [ $same -gt 0 ]; then
			echo "HTML: the same both ways ($same pages, $bytes bytes)"
		else
			echo "HTML: $differ of $((same + differ)) pages differ"
			status=1
		fi
		# render.c's line of RV_STATS, a timed render
		old0=$(sed -n 's/.*dispatches new \([0-9]*\) old \([0-9]*\),.*/\2/p' "$out/$name-0/render.err")
		sed -n 's/.*dispatches new \([0-9]*\) old \([0-9]*\), calls \(.*\)/\1 \2 \3/p' "$out/$name-1/render.err" |
			while read -r new old rest; do
				all=$((new + old))
				echo "register loop on: $new dispatches a render in the register loop, $old in the old loop: $(awk "BEGIN { printf \"%.2f%%\", 100 * $new / $all }") in the register loop"
				echo "  (register loop off: $old0 dispatches a render, all in the old loop)"
				echo "  calls a render: $rest"
			done
		# without the records of the functions freed during the renders
		grep -v "^[FVBCK] [0-9]" "$out/$name-1/score.txt" || true
		echo
	} >> "$board"
}

page react "$root/compare/engines/react/react-ssr.js" "$out/react.json" "$out/react-start.js" 0
if [ -n "${BENCH_DIR:-}" ]; then
	for p in table spiral; do
		page "$p" "$BENCH_DIR/bundles/bench.js" "$BENCH_DIR/in/bench/$p.json" "$BENCH_DIR/prelude/iterator-shim.js" 1
	done
else
	echo "(BENCH_DIR not set: the bench pages are left out)" >> "$board"
fi
# the apps of tifa (shop and mui): APPS_DIR holds prelude/fakeintl.js,
# APP/bundles/APP.js and APP/in/PAGE.json, every page a render
if [ -n "${APPS_DIR:-}" ]; then
	for app in shop mui; do
		inputs=$(ls "$APPS_DIR/$app/in/"*.json | tr '\n' , | sed 's/,$//')
		page "$app" "$APPS_DIR/$app/bundles/$app.js" "$inputs" "$APPS_DIR/prelude/fakeintl.js" 1
	done
fi
cat "$board"
exit $status
