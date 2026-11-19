#!/bin/sh
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE at the
# root of this repository.
#
# Which hooks of the throw injection (engine/regvm-inject.h) the scripts of
# inject.sh come to, and which none does: the places in the handlers of the
# register loop and their slow sides that no injection of the set throws
# at. Each script runs three times through rvdiff of a build with
# -DRV_INJECT, as inject.sh runs it: once to count its places, then twice in
# the cover mode (RV_INJECT_COVER), which throws nowhere: each instruction
# is armed at its first place, then at its second, where inject.sh injects
# (every place, or the first M over the number of sites at each site for a
# script with more than M places), and the hook the injection would throw at
# is written down. The scripts G/skip names are left out, as inject.sh
# leaves them out.
#
#   engine/test/regvm/inject-cover.sh [-j N] [-max M] [-golden G] DIR PATH...
#
# The hooks of the engine are those the preprocessor gives in quickjs.c
# built as DIR is: each expansion of RV_INJ_HIT, the reading of the
# interrupt flag of a jump among them. A hook is named by the file and line
# the expansion starts at, and by #K when that line has more than one (a
# macro of a handler with two hooks): the K-th of them. It prints:
#
#   hook HOOK OP SUB SCRIPTS  a hook taken under the op OP (SUB the old
#                             opcode under OLD, WITH and W_GEN, or -), by
#                             SCRIPTS scripts
#   none HOOK                 a hook of the engine no script comes to
#   miss OP SUB SCRIPT        an instruction that ran to its end with no
#                             hook taken: inject.sh fails at it
#   outside OP SUB HOOK SCRIPT a hook of JS the instruction ran, taken while
#                             it was armed
#
# and a last line with the counts. The lines of each script are in
# DIR/cover/SCRIPT.cover. It exits with 1 when a script did not run.
set -eu

jobs=4
max=2000
golden=$(cd "$(dirname "$0")" && pwd)/golden/inject
single=
while [ $# -gt 0 ]; do
	case $1 in
	-j) jobs=$2; shift 2 ;;
	-max) max=$2; shift 2 ;;
	-golden) golden=$2; shift 2 ;;
	-one) single=1; shift ;; # -one DIR SCRIPT: one script, for xargs
	-*) echo "usage: inject-cover.sh [-j N] [-max M] [-golden G] DIR PATH..." >&2; exit 2 ;;
	*) break ;;
	esac
done
if [ $# -lt 2 ]; then
	echo "usage: inject-cover.sh [-j N] [-max M] [-golden G] DIR PATH..." >&2
	exit 2
fi
here=$(cd "$(dirname "$0")" && pwd)
engine=$(dirname "$(dirname "$here")")
dir=$(cd "$1" && pwd)
shift
rvdiff=$dir/rvdiff
work=$dir/cover
ASAN_OPTIONS=${ASAN_OPTIONS:+$ASAN_OPTIONS:}detect_leaks=0
export ASAN_OPTIONS

if [ -n "$single" ]; then
	sc=$1
	base=$(basename "$sc")
	out=$work/$base
	rm -rf "$out.d" "$out.count" "$out.cover"
	mkdir -p "$out.d"
	RV_INJECT=2000000000 RV_INJECT_OUT="$out.count" "$rvdiff" -q -golden "$out.d" -update -root "$(dirname "$sc")" "$sc" > /dev/null 2>&1 || true
	total=$(sed -n 's/^none \([0-9]*\) [0-9]*$/\1/p' "$out.count" 2>/dev/null | sort -n | tail -1)
	sites=$(sed -n 's/^none [0-9]* \([0-9]*\)$/\1/p' "$out.count" 2>/dev/null | sort -n | tail -1)
	rm -f "$out.count"
	if [ -z "$total" ] || [ "$total" = 0 ]; then
		echo "FAIL $sc: no place to throw ran in the count run"
		rm -rf "$out.d"
		exit 0
	fi
	each=0
	if [ "$total" -gt "$max" ]; then
		each=$((max / sites))
		[ "$each" -ge 1 ] || each=1
	fi
	: > "$out.raw"
	for place in 1 2; do
		rm -rf "$out.d"
		mkdir -p "$out.d"
		RV_INJECT_COVER="$each $place" RV_INJECT_OUT="$out.raw" "$rvdiff" -q -golden "$out.d" -update -root "$(dirname "$sc")" "$sc" > /dev/null 2>&1 || true
	done
	rm -rf "$out.d"
	if [ "$(grep -c '^none ' "$out.raw")" != 2 ]; then
		echo "FAIL $sc: a cover run wrote nothing"
		rm -f "$out.raw"
		exit 0
	fi
	if grep -q '^lost$' "$out.raw"; then
		echo "FAIL $sc: the table of hooks was full"
	fi
	grep -E '^(cover|miss|outside) ' "$out.raw" | LC_ALL=C sort -u > "$out.cover" || true
	rm -f "$out.raw"
	exit 0
fi

scripts=
for p in "$@"; do
	if [ -d "$p" ]; then
		for f in "$p"/*.js; do
			[ -f "$f" ] || continue
			scripts="$scripts $f"
		done
	elif [ -f "$p" ]; then
		scripts="$scripts $p"
	else
		echo "inject-cover.sh: no script $p" >&2
		exit 2
	fi
done
mkdir -p "$work"
list=$work/scripts
: > "$list"
for sc in $scripts; do
	base=$(basename "$sc")
	if grep -q "^$base " "$golden/skip" 2>/dev/null; then
		echo "SKIP $sc"
		continue
	fi
	echo "$sc" >> "$list"
done
fails=$(xargs -P "$jobs" -I{} "$0" -golden "$golden" -max "$max" -one "$dir" {} < "$list")
[ -z "$fails" ] || echo "$fails"

# every hook of the engine as DIR built it: "ID NAME", the name FILE:LINE,
# or FILE:LINE#K for the K-th hook at a line with more than one
. "$engine/test/flags.sh"
cc=${CC:-clang}
# shellcheck disable=SC2086
$cc -E $engine_flags $(engine_target_flags native) -DJS_HOST_POLL -DRV_SELF_CHECK -DRV_INJECT \
	-DCONFIG_PRINT_VALUE -I"$engine/libc" -I"$engine" -DCONFIG_VERSION=\"$engine_version\" "$engine/quickjs.c" |
	grep -o 'rv_inj_hit(ctx, "[^"]*", [0-9]*, [0-9]*)' |
	sed 's|rv_inj_hit(ctx, "\(.*/\)\{0,1\}\([^"/]*\)", \([0-9]*\), \([0-9]*\))|\2:\3 \4|' |
	LC_ALL=C sort -u | sort -k1,1 -k2,2n |
	awk '{ at[NR] = $1; id[NR] = $2; n[$1]++ }
	END { for (i = 1; i <= NR; i++) { k[at[i]]++; print id[i], (n[at[i]] > 1 ? at[i] "#" k[at[i]] : at[i]) } }' > "$work/hooks"

# the lines of the scripts, the hook by its name: KIND OP SUB [HOOK] SCRIPT
all=$work/all
: > "$all"
while read -r sc; do
	base=$(basename "$sc")
	[ -f "$work/$base.cover" ] || continue
	sed "s|\$| $base|" "$work/$base.cover" >> "$all"
done < "$list"
awk 'FILENAME == ARGV[1] { name[$1] = $2; next }
	NF == 5 { n = split($4, f, ":"); $4 = (f[n] in name) ? name[f[n]] : $4 "?" }
	{ print }' "$work/hooks" "$all" > "$all.named"
awk '$1 == "cover" { n[$4 " " $2 " " $3]++ } END { for (k in n) print k, n[k] }' "$all.named" |
	LC_ALL=C sort | sed 's/^/hook /'
awk '$1 == "cover" { print $4 }' "$all.named" | LC_ALL=C sort -u > "$work/reached"
cut -d' ' -f2 "$work/hooks" | LC_ALL=C sort -u > "$work/names"
LC_ALL=C comm -23 "$work/names" "$work/reached" | sed 's/^/none /'
grep -E '^(miss|outside) ' "$all.named" || true
echo "inject-cover: $(wc -l < "$list" | tr -d ' ') scripts, $(wc -l < "$work/names" | tr -d ' ') hooks, $(LC_ALL=C comm -12 "$work/names" "$work/reached" | wc -l | tr -d ' ') reached, $(LC_ALL=C comm -23 "$work/names" "$work/reached" | wc -l | tr -d ' ') reached by none"
[ -z "$fails" ]
