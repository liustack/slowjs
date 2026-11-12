#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Build the tools that run the native test host with the register loop off
# and on, against the objects engine/test/build.sh made:
#
#   engine/test/regvm/build.sh [MODE] [OUTDIR]
#
# MODE is plain (default), asan, asan-heap, ubsan or asan-ubsan, and OUTDIR
# the folder of that build (default engine/test/out/MODE). When OUTDIR holds
# no engine, or one of other sources than the tree has now, another mode,
# other EXTRA_DEFS or another CC (its engine.stamp, see engine/test/stamp.sh),
# engine/test/build.sh MODE OUTDIR builds it again first (EXTRA_DEFS passes
# on, -DRV_STATS for the counts of each loop). It adds to OUTDIR:
#
#   rvdiff            the differential runner (rvdiff.c)
#   rvops             the check of the table of opcodes (rvops.c): rvops
#                     engine/test/regvm/opcodes.txt
#   run-test262-rv    run-test262, unit-rv, unit-oom-rv and unit-api-rv: the
#   unit-rv ...       programs of engine/test with every context they make
#                     turning the register loop on when RV_REGVM=1
#                     (rvhook.h), the same programs otherwise
#   regvm.stamp       what all of OUTDIR was built from: the scripts that run
#                     these tools refuse a folder whose stamp is not of the
#                     tree now. rvdiff prints it too, as "engine ..." in the
#                     info lines of a transcript and before its summary
set -eu

here=$(cd "$(dirname "$0")" && pwd)
test=$(dirname "$here")
engine=$(dirname "$test")
mode=${1:-plain}
out=${2:-$test/out/$mode}
cc=${CC:-clang}

defs=
system=0
case $mode in
plain) san= ;;
asan) san='-fsanitize=address -fsanitize-recover=address -fno-omit-frame-pointer'; defs=-DJS_HEAP_SYSTEM_MALLOC; system=1 ;;
asan-heap) san='-fsanitize=address -fsanitize-recover=address -fno-omit-frame-pointer'; defs=-DJS_HEAP_POISON ;;
ubsan) san='-fsanitize=undefined -fno-omit-frame-pointer' ;;
asan-ubsan) san='-fsanitize=address,undefined -fsanitize-recover=address -fno-omit-frame-pointer'; defs=-DJS_HEAP_SYSTEM_MALLOC; system=1 ;;
*) echo "regvm/build.sh: unknown mode $mode (plain, asan, asan-heap, ubsan or asan-ubsan)" >&2; exit 2 ;;
esac
defs="$defs ${EXTRA_DEFS:-}"

. "$test/stamp.sh"
rm -f "$out/regvm.stamp"
tools=$(stamp_tools "$engine")
if ! stamp_engine_fresh "$engine" "$out" "$mode" "$cc"; then
	echo "regvm/build.sh: building the engine of $out (engine/test/build.sh $mode), since its engine.stamp is not of the tree, mode, EXTRA_DEFS and CC of now" >&2
	mkdir -p "$(dirname "$out")"
	"$test/build.sh" "$mode" "$out" > "$out.build.log" 2>&1 || {
		echo "regvm/build.sh: engine/test/build.sh $mode failed, see $out.build.log" >&2
		exit 1
	}
fi

. "$test/flags.sh"
# what rvdiff prints of its build: the engine it links, and the tools
rvstamp="$(sed -n 's/^src //p' "$out/engine.stamp") tools $tools mode $mode${EXTRA_DEFS:+ defs $EXTRA_DEFS}"
common="-O2 -g -Wall -Wno-array-bounds $san"
hostflags="$common -fwrapv -funsigned-char -D_GNU_SOURCE -DCONFIG_VERSION=\"$engine_version\" -DCONFIG_PRINT_VALUE -I$engine -I$test $defs"

objs="$out/quickjs.o $out/libregexp.o $out/libunicode.o $out/cutils.o $out/dtoa.o"
if [ $system = 1 ]; then
	objs="$objs $out/heap-system.o"
else
	objs="$objs $out/heap.o $out/tlsf.o"
fi
objs="$objs $out/libc/*.o $out/host.o"

# shellcheck disable=SC2086
$cc $hostflags -Wno-unused-function -DRV_ENGINE_STAMP="\"src $rvstamp\"" -DRV_ENGINE_DIR="\"$engine\"" -c "$here/rvdiff.c" -o "$out/rvdiff.o"
# shellcheck disable=SC2086
$cc $san $objs "$out/rvdiff.o" -o "$out/rvdiff" -lpthread
# shellcheck disable=SC2086
$cc $hostflags -c "$here/rvops.c" -o "$out/rvops.o"
# shellcheck disable=SC2086
$cc $san $objs "$out/rvops.o" -o "$out/rvops" -lpthread

for p in run-test262 unit unit-oom unit-api; do
	name=$p-rv
	[ "$p" = run-test262 ] && name=run-test262-rv
	# shellcheck disable=SC2086
	$cc $hostflags -include "$here/rvhook.h" -c "$test/$p.c" -o "$out/$name.o"
	# shellcheck disable=SC2086
	$cc $san $objs "$out/$name.o" -o "$out/$name" -lpthread
done
{ cat "$out/engine.stamp"; echo "tools $tools"; } > "$out/regvm.stamp"
echo "$out/rvdiff"
