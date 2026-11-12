#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# The throw-injection test of the register loop (engine/regvm-inject.h): for
# each script, for N = 1, 2, 3 ... up to the number of places where the
# instructions the register loop runs in it can throw, the N-th of them
# throws an InternalError: the handler of the instruction runs and fails at
# the place where it calls into the engine, so what it frees then and what
# its pcmap entry tells the unwinding to free are two things written apart,
# and a temp freed twice or not at all shows. Each run goes through rvdiff
# (see rvdiff.c) twice:
#
#   1. RV_INJECT=N: the run with the register loop on throws, the other does
#      not. Only the status of the two is compared: a leak found when the
#      runtime is freed (87) or a sanitizer report (86) is a handler and a
#      pcmap entry that do not agree on a temp. The run also says where the
#      throw was: an old instruction and how many times it ran. An
#      instruction that was to throw and ran to its end ("miss"), or whose
#      throw a hook of JS it ran took ("outside"), is a hole of the test and
#      fails the run.
#   2. When that is defined (the first register instruction of its old
#      instruction, thrown at by itself and not at the interrupt flag of a
#      conditional jump), RV_INJECT=N and RV_INJECT_AT=that place: the old
#      loop throws the same error at the same run of the same old
#      instruction, and everything is compared: output, result, exception,
#      stack and the counts of the runtime after a collection.
#
# A run that throws also says at which hook: "hit NAME P", the P-th place
# of the register instruction NAME. A script that has a line
# "// inject-hit: NAME P" fails when no run threw there, so that a place a
# test is written for cannot be lost.
#
#   engine/test/regvm/inject.sh [-j N] [-max M] [-record G | -golden G] DIR PATH...
#
# DIR holds rvdiff of a build with -DRV_INJECT:
#
#   EXTRA_DEFS=-DRV_INJECT engine/test/build.sh MODE engine/test/out/inject-MODE
#   EXTRA_DEFS=-DRV_INJECT engine/test/regvm/build.sh MODE engine/test/out/inject-MODE
#
# PATH is a script or a folder of scripts. -j runs N at a time (default 4).
# A script with more than M (default 2000) places run is sampled by site: at
# each old instruction of each function the register loop runs, the first
# places run there, M over the number of sites of them and one at least, so
# that an instruction run a few times gets its share beside a hot loop. It prints one line per
# run that differs and a line for each script, and exits with 1 when any
# run differs. The transcripts of a run that differs are in
# DIR/inject/SCRIPT-N/.
#
# -record G also writes, for each script, G/SCRIPT.txt: a line for each run
# of step 2 where the two loops agree: the place of the throw
# (LINE_COL_POS_K), the injection (N or S.J) and the status, count and
# checksum of each kind of the old loop's run. -golden G is for when there is
# one loop only: each run is made once, with the register loop on, the
# injections G/SCRIPT.txt names as well as those sampled here, and a run at a
# place G/SCRIPT.txt has is compared with that line. The run with no
# injection must end with exit 0 or 1. A run at a place with no line, or
# with no place, is judged by its status: it must end with exit 0 or 1 (an
# uncaught throw) and say what it did. A leak (87), a sanitizer report, a
# signal (an abort of a self check), the time limit, a "miss" or an
# "outside" is a difference. The places hold while the bytecode of the
# script is the same.
set -eu

self=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")
jobs=4
max=2000
single=
record=
golden=
while [ $# -gt 0 ]; do
	case $1 in
	-j) jobs=$2; shift 2 ;;
	-max) max=$2; shift 2 ;;
	-record) record=$2; shift 2 ;;
	-golden) golden=$2; shift 2 ;;
	-one) single=1; shift ;; # -one DIR N SCRIPT: one run, for xargs
	*) break ;;
	esac
done
if [ $# -lt 2 ] || { [ -n "$record" ] && [ -n "$golden" ]; }; then
	echo "usage: inject.sh [-j N] [-max M] [-record G | -golden G] DIR PATH..." >&2
	exit 2
fi
dir=$(cd "$1" && pwd)
shift
rvdiff=$dir/rvdiff
[ -x "$rvdiff" ] || { echo "inject.sh: no $rvdiff" >&2; exit 2; }
# a build of the tree now (see engine/test/stamp.sh), checked once: the runs of
# -one take RV_TREE_STAMP from it
if [ -z "$single" ]; then
	. "$(dirname "$0")/../stamp.sh"
	stamp_check "$(cd "$(dirname "$0")/../.." && pwd)" "$dir" || exit 2
fi
work=$dir/inject
mkdir -p "$work"
# rvExpect is left out of both runs: a function that the throw kept from
# its second call has no register code, which is no fault of the loop
status_only=out,result,exception,stack,job,job-stack,mem-end,mem-gc,expect
# LeakSanitizer (on by default with ASan on linux) would fail every run on
# the blocks rvdiff itself keeps to its exit. A leak of the engine still
# shows: the runtime counts its blocks when it is freed (87).
ASAN_OPTIONS=${ASAN_OPTIONS:+$ASAN_OPTIONS:}detect_leaks=0
export ASAN_OPTIONS

# the line of a golden file of rvdiff (see rvdiff.c): its status, and the
# count and checksum of each kind but expect (the hash of a kind kept as one)
summary() {
	s="status=$(sed -n 's/^--- status //p' "$1" | tr ' ' _)"
	for k in out result exception stack job job-stack mem-end mem-gc; do
		h=$(grep "^hash $k " "$1" | cut -d' ' -f3,4 | tr ' ' /)
		if [ -n "$h" ]; then
			s="$s $k=fnv$h"
		elif grep -q "^$k " "$1"; then
			s="$s $k=$(grep "^$k " "$1" | cksum | cut -d' ' -f1)/$(grep -c "^$k " "$1")"
		fi
	done
	echo "$s"
}

# one_run DIR KEY VAR=VAL: the run with the register loop on alone, its
# transcript as its golden to the folder DIR (rvdiff -golden of an empty
# folder with -update)
one_run() {
	rm -rf "$1"
	mkdir -p "$1"
	env "$3" RV_INJECT_OUT="$2" "$rvdiff" -q -golden "$1" -update -root "$(dirname "$sc")" "$sc" > "$1.log" 2>&1 || true
}

if [ -n "$single" ]; then
	n=$1 sc=$2
	name=$(basename "$sc")-$n
	key=$work/$name.key
	rm -f "$key"
	# N, or S.J: the J-th place run at site S
	case $n in
	*.*) var=RV_INJECT_SITE val="${n%.*} ${n#*.}" ;;
	*) var=RV_INJECT val=$n ;;
	esac
	if [ -n "$golden" ]; then
		base=$(basename "$sc")
		one_run "$work/$name" "$key" "$var=$val"
		grep "^hit " "$key" >> "$work/$base.hits" 2>/dev/null || true
		at=$(grep -E '^(at|nokey|real|skip|miss|outside)' "$key" 2>/dev/null || echo '?')
		g=$(find "$work/$name" -name "*.txt" | head -1)
		if [ -z "$g" ]; then
			echo "DIFF $sc N=$n status ($at): $(grep '^NOUPDATE' "$work/$name.log" | cut -c1-200)"
			exit 0
		fi
		place=$(echo "$at" | sed -n 's/^at //p' | tr ' ' _)
		want=
		[ -z "$place" ] || want=$(grep "^$place " "$golden/$base.txt" 2>/dev/null | cut -d' ' -f3- || true)
		got=$(summary "$g")
		if echo "$at" | grep -q '^miss'; then
			echo "DIFF $sc N=$n $at: the instruction ran to its end, no hook of its handler threw"
		elif echo "$at" | grep -q '^outside'; then
			echo "DIFF $sc N=$n $at: a hook of JS the instruction ran was taken for it"
		elif [ -n "$want" ] && [ "$want" != "$got" ]; then
			echo "DIFF $sc N=$n $at: golden $want, run $got"
		elif [ ! -s "$key" ]; then
			echo "DIFF $sc N=$n status (the run wrote no result of the injection): $(sed -n 's/^--- status //p' "$g")"
		elif [ -z "$want" ] && ! grep -q '^--- status exit [01]$' "$g"; then
			echo "DIFF $sc N=$n status ($at): $(sed -n 's/^--- status //p' "$g")"
		else
			rm -rf "$work/$name" "$work/$name.log"
		fi
		rm -f "$key"
		exit 0
	fi
	rc=0
	env "$var=$val" RV_INJECT_OUT="$key" "$rvdiff" -q -ignore $status_only \
		-o "$work/$name" "$sc" > /dev/null 2>&1 || rc=1
	grep "^hit " "$key" >> "$work/$(basename "$sc").hits" 2>/dev/null || true
	if [ $rc != 0 ]; then
		echo "DIFF $sc N=$n status ($(grep -E '^(at|nokey|real|skip|miss|outside)' "$key" 2>/dev/null || echo '?'))"
		exit 0
	fi
	if grep -q '^outside' "$key" 2>/dev/null; then
		echo "DIFF $sc N=$n $(grep '^outside' "$key"): a hook of JS the instruction ran was taken for it"
		rm -f "$key"
		exit 0
	fi
	if grep -q '^miss' "$key" 2>/dev/null; then
		echo "DIFF $sc N=$n $(grep '^miss' "$key"): the instruction ran to its end, no hook of its handler threw"
		rm -f "$key"
		exit 0
	fi
	at=$(grep '^at ' "$key" || true)
	rm -f "$key"
	[ -n "$at" ] || exit 0
	rec=
	[ -z "$record" ] || { rec=$work/$name.rec; rm -rf "$rec"; mkdir -p "$rec"; }
	if ! env "$var=$val" RV_INJECT_AT="$at" "$rvdiff" -q -ignore expect -o "$work/$name" \
		${rec:+-record "$rec" -root "$(dirname "$sc")"} "$sc" > "$work/$name.log" 2>&1; then
		echo "DIFF $sc N=$n $at: $(tail -1 "$work/$name.log")"
		exit 0
	fi
	if [ -n "$rec" ]; then
		f=$(find "$rec" -name "*.txt" | head -1)
		[ -z "$f" ] || echo "$(echo "$at" | sed "s/^at //" | tr " " _) $n $(summary "$f")" > "$work/$name.gline"
		rm -rf "$rec"
	fi
	rm -f "$work/$name.log"
	exit 0
fi

scripts=
for p in "$@"; do
	if [ -d "$p" ]; then
		for f in "$p"/*.js; do
			if [ -f "$f" ]; then scripts="$scripts $f"; fi
		done
	else
		scripts="$scripts $p"
	fi
done

status=0
for sc in $scripts; do
	base=$(basename "$sc")
	key=$work/$base.count
	rm -f "$key"
	if [ -n "$golden" ]; then
		one_run "$work/$base.count.d" "$key" RV_INJECT=2000000000
		# with no injection the run must end as a script does
		c=$(find "$work/$base.count.d" -name "*.txt" | head -1)
		if [ -z "$c" ] || ! grep -q '^--- status exit [01]$' "$c"; then
			echo "DIFF $sc: the run with no injection ends with $( [ -n "$c" ] && sed -n 's/^--- status //p' "$c" || grep '^NOUPDATE' "$work/$base.count.d.log" | cut -c1-200)"
			status=1
		fi
		rm -rf "$work/$base.count.d" "$work/$base.count.d.log"
	else
		RV_INJECT=2000000000 RV_INJECT_OUT=$key "$rvdiff" -q -ignore $status_only \
			"$sc" > /dev/null 2>&1 || true
	fi
	total=$(sed -n 's/^none \([0-9]*\) [0-9]*$/\1/p' "$key" 2>/dev/null | sort -n | tail -1)
	sites=$(sed -n 's/^none [0-9]* \([0-9]*\)$/\1/p' "$key" 2>/dev/null | sort -n | tail -1)
	rm -f "$key"
	[ -n "$total" ] || total=0
	[ -n "$sites" ] || sites=0
	if [ "$total" = 0 ]; then
		# the count run failed, or no place of the register loop ran: the
		# script tests nothing, a hole of the test
		echo "DIFF $sc: no place to throw ran in the count run"
		echo "inject: $sc: 0 places to throw run at 0 sites, 0 injected, 1 differ"
		status=1
		continue
	fi
	if [ "$total" -le "$max" ]; then
		ns=$(seq 1 "$total")
	else
		each=$((max / sites))
		[ "$each" -ge 1 ] || each=1
		ns=$(awk -v s="$sites" -v q="$each" \
			'BEGIN { for (i = 1; i <= s; i++) for (j = 1; j <= q; j++) print i "." j }')
	fi
	# with -golden, also each injection recorded, so that its place is compared
	# even when the sampling here picks others
	if [ -n "$golden" ] && [ -f "$golden/$base.txt" ]; then
		ns=$(printf '%s\n' $ns $(cut -d' ' -f2 "$golden/$base.txt") | LC_ALL=C sort -u)
	fi
	runs=$(echo $ns | wc -w | tr -d ' ')
	hits=$work/$(basename "$sc").hits
	rm -f "$hits"
	out=$work/$base.diffs
	rm -f "$work/$base"-*.gline
	# shellcheck disable=SC2086
	printf '%s\n' $ns | xargs -P "$jobs" -I{} "$self" ${record:+-record "$record"} ${golden:+-golden "$golden"} \
		-one "$dir" {} "$sc" > "$out" || true
	# the places the script says a run must throw at: "// inject-hit: NAME P"
	sed -n "s|^// inject-hit: \(.*\)$|hit \1|p" "$sc" | while read -r want; do
		grep -qxF "$want" "$hits" 2>/dev/null ||
			echo "DIFF $sc: no run threw at \"${want#hit }\""
	done >> "$out"
	rm -f "$hits"
	bad=$(wc -l < "$out" | tr -d ' ')
	cat "$out"
	echo "inject: $sc: $total places to throw run at $sites sites, $runs injected, $bad differ"
	if [ -n "$record" ]; then
		mkdir -p "$record"
		find "$work" -name "$base-*.gline" -exec cat {} + 2>/dev/null | LC_ALL=C sort -u > "$record/$base.txt"
		rm -f "$work/$base"-*.gline
		echo "inject: $sc: $(wc -l < "$record/$base.txt" | tr -d ' ') places recorded in $record/$base.txt"
	fi
	[ "$bad" = 0 ] || status=1
done
exit $status
