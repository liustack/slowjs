#!/bin/sh
# Copyright (c) 2026 The liustack/beastjs Authors.
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
# (every place, or for a script with more than M places the first M over
# the number of sites at each site and the places its golden G/SCRIPT.txt
# names), and the hook the injection would throw at is written down. Each
# run must end as a script does (exit 0 or 1, as inject.sh wants of its run
# with no injection): a leak, a signal or a sanitizer that stops the run
# fails the script. The scripts G/skip names are left out, as inject.sh
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
# DIR/cover/NAME.cover, NAME its path with % and / written %25 and %2F. It
# exits with 1 when a script failed, and with 2 when the preprocessor gives
# no hooks.
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

# name_of PATH: the name of the files of the script PATH in DIR/cover, one
# for each path, so that two scripts of one name in two folders keep apart
name_of() {
	printf '%s\n' "$1" | sed 's|%|%25|g; s|/|%2F|g'
}

if [ -n "$single" ]; then
	sc=$1
	base=$(basename "$sc")
	out=$work/$(name_of "$sc")
	rm -rf "$out.d" "$out.count" "$out.cover" "$out.raw" "$out.at" "$out.log"
	# run VAR=VAL...: one run of the script, its transcript in $out.d. It
	# fails the script unless the run ends as a script does, as inject.sh
	# wants of its run with no injection (a leak of the engine is status 87)
	run() {
		rm -rf "$out.d"
		mkdir -p "$out.d"
		env "$@" "$rvdiff" -q -golden "$out.d" -update -root "$(dirname "$sc")" "$sc" > "$out.log" 2>&1 || true
		g=$(find "$out.d" -name "*.txt" | head -1)
		if [ -z "$g" ] || ! grep -q '^--- status exit [01]$' "$g"; then
			echo "FAIL $sc: the run with $1 ends with $( [ -n "$g" ] && sed -n 's/^--- status //p' "$g" || grep '^NOUPDATE' "$out.log" | cut -c1-200)"
			rm -rf "$out.d" "$out.count" "$out.raw" "$out.at" "$out.log"
			exit 0
		fi
	}
	run RV_INJECT=2000000000 RV_INJECT_OUT="$out.count"
	total=$(sed -n 's/^none \([0-9]*\) [0-9]*$/\1/p' "$out.count" 2>/dev/null | sort -n | tail -1)
	sites=$(sed -n 's/^none [0-9]* \([0-9]*\)$/\1/p' "$out.count" 2>/dev/null | sort -n | tail -1)
	rm -f "$out.count"
	if [ -z "$total" ] || [ "$total" = 0 ]; then
		echo "FAIL $sc: no place to throw ran in the count run"
		rm -rf "$out.d" "$out.log"
		exit 0
	fi
	# the places inject.sh runs: every place, or the first max / sites at
	# each site and each place its golden names
	each=0
	: > "$out.at"
	if [ "$total" -gt "$max" ]; then
		each=$((max / sites))
		[ "$each" -ge 1 ] || each=1
		[ ! -f "$golden/$base.txt" ] || cut -d' ' -f2 "$golden/$base.txt" > "$out.at"
	fi
	: > "$out.raw"
	for place in 1 2; do
		run RV_INJECT_COVER="$each $place" RV_INJECT_COVER_AT="$out.at" RV_INJECT_OUT="$out.raw"
	done
	rm -rf "$out.d" "$out.at" "$out.log"
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
# quickjs.c as DIR built it, to a file first, so that set -e sees the
# preprocessor fail
. "$engine/test/flags.sh"
cc=${CC:-clang}
# shellcheck disable=SC2086
$cc -E $engine_flags $(engine_target_flags native) -DJS_HOST_POLL -DRV_SELF_CHECK -DRV_INJECT \
	-DCONFIG_PRINT_VALUE -I"$engine/libc" -I"$engine" -DCONFIG_VERSION=\"$engine_version\" \
	"$engine/quickjs.c" > "$work/quickjs.i"
fails=$(xargs -P "$jobs" -I{} "$0" -golden "$golden" -max "$max" -one "$dir" {} < "$list")
[ -z "$fails" ] || echo "$fails"

# every hook of the engine as DIR built it: "ID NAME", the name FILE:LINE,
# or FILE:LINE#K for the K-th hook at a line with more than one
grep -o 'rv_inj_hit(ctx, "[^"]*", [0-9]*, [0-9]*)' "$work/quickjs.i" |
	sed 's|rv_inj_hit(ctx, "\(.*/\)\{0,1\}\([^"/]*\)", \([0-9]*\), \([0-9]*\))|\2:\3 \4|' |
	LC_ALL=C sort -u | sort -k1,1 -k2,2n |
	awk '{ at[NR] = $1; id[NR] = $2; n[$1]++ }
	END { for (i = 1; i <= NR; i++) { k[at[i]]++; print id[i], (n[at[i]] > 1 ? at[i] "#" k[at[i]] : at[i]) } }' > "$work/hooks"
rm -f "$work/quickjs.i"
if [ ! -s "$work/hooks" ]; then
	echo "inject-cover.sh: the preprocessor gives no hook of the injection in $engine/quickjs.c" >&2
	exit 2
fi

# the lines of the scripts, the hook by its name: KIND OP SUB [HOOK] SCRIPT
all=$work/all
: > "$all"
while read -r sc; do
	f=$work/$(name_of "$sc").cover
	[ -f "$f" ] || continue
	awk -v s="$sc" '{ print $0, s }' "$f" >> "$all"
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
