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
# and a temp freed twice or not at all shows. Each run (RV_INJECT=N) goes
# through rvdiff (see rvdiff.c) once and says where the throw was: a
# bytecode instruction and how many times it ran (its place). A run at a
# place the goldens have is compared with the old loop's run with the same
# throw, recorded at deb70815 while the old loop existed: the status, and
# the count and checksum of each kind (output, result, exception, stack,
# job, the counts of the runtime after a collection). A run at a place with
# no golden, or with no place, is judged by its status: it must end with
# exit 0 or 1 (an uncaught throw). A leak found when the runtime is freed
# (87), a sanitizer report (86), a signal (an abort of a self check), the
# time limit, an instruction that was to throw and ran to its end ("miss"),
# or whose throw a hook of JS it ran took ("outside"), is a difference. The
# run with no injection must end with exit 0 or 1.
#
# A run that throws also says at which hook: "hit NAME P", the P-th place
# of the register instruction NAME. A script that has a line
# "// inject-hit: NAME P" fails when no run threw there, so that a place a
# test is written for cannot be lost.
#
#   engine/test/regvm/inject.sh [-j N] [-max M] [-golden G] [-update] DIR PATH...
#   engine/test/regvm/inject.sh [-golden G] -goldens PATH...
#
# DIR holds rvdiff of a build with -DRV_INJECT:
#
#   EXTRA_DEFS=-DRV_INJECT engine/test/build.sh MODE engine/test/out/inject-MODE
#   EXTRA_DEFS=-DRV_INJECT engine/test/regvm/build.sh MODE engine/test/out/inject-MODE
#
# PATH is a script or a folder of scripts. -j runs N at a time (default 4).
# A script with more than M (default 2000) places run is sampled by site: at
# each bytecode instruction of each function the register loop runs, the
# first places run there, M over the number of sites of them and one at
# least, so that an instruction run a few times gets its share beside a hot
# loop. The injections G/SCRIPT.txt names are run as well, so that each
# recorded place is compared whatever the sampling picks. G is
# engine/test/regvm/golden/inject by default. A script with no G/SCRIPT.txt
# differs, unless G/skip names it on a line "SCRIPT WHY" (a script with too
# many places to inject in the time of a test): it prints "SKIP SCRIPT: WHY"
# and runs nothing. -update writes G/SCRIPT.txt again from the run, for a new
# script or a change of output that is meant: read the diff before it goes
# into a commit. The places hold while the
# bytecode of the script is the same. It prints one line per run that
# differs and a line for each script, and exits with 1 when any run
# differs. The transcripts of a run that differs are in DIR/inject/SCRIPT-N/.
#
# -goldens builds and runs nothing: it names each script of PATH with neither
# a G/SCRIPT.txt nor a line in G/skip, and exits with 1 when there is one, so
# that a script added without its golden shows before the injections run.
set -eu

self=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")
jobs=4
max=2000
single=
update=
goldens=
golden=$(cd "$(dirname "$0")" && pwd)/golden/inject
while [ $# -gt 0 ]; do
	case $1 in
	-j) jobs=$2; shift 2 ;;
	-max) max=$2; shift 2 ;;
	-golden) golden=$2; shift 2 ;;
	-update) update=1; shift ;;
	-goldens) goldens=1; shift ;;
	-one) single=1; shift ;; # -one DIR N SCRIPT: one run, for xargs
	-*) echo "usage: inject.sh [-j N] [-max M] [-golden G] [-update] DIR PATH..., or inject.sh [-golden G] -goldens PATH..." >&2; exit 2 ;;
	*) break ;;
	esac
done
if [ -n "$goldens" ]; then
	n=0 bad=0
	for p in "$@"; do
		if [ -d "$p" ]; then list=$(ls "$p"/*.js); else list=$p; fi
		for sc in $list; do
			n=$((n + 1))
			base=$(basename "$sc")
			[ ! -f "$golden/$base.txt" ] || continue
			! grep -q "^$base " "$golden/skip" 2>/dev/null || continue
			echo "DIFF $sc: no golden $golden/$base.txt and no line in $golden/skip (-update writes it from the run, read it first)"
			bad=$((bad + 1))
		done
	done
	echo "inject: $n scripts, $bad without a golden"
	[ "$bad" = 0 ] && exit 0
	exit 1
fi
if [ $# -lt 2 ]; then
	echo "usage: inject.sh [-j N] [-max M] [-golden G] [-update] DIR PATH..., or inject.sh [-golden G] -goldens PATH..." >&2
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
	[ -d "$golden" ] || [ -n "$update" ] || { echo "inject.sh: no goldens in $golden (-golden G names others, -update writes them)" >&2; exit 2; }
	mkdir -p "$golden"
fi
work=$dir/inject
mkdir -p "$work"
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
	base=$(basename "$sc")
	name=$base-$n
	key=$work/$name.key
	rm -f "$key"
	# N, or S.J: the J-th place run at site S
	case $n in
	*.*) var=RV_INJECT_SITE val="${n%.*} ${n#*.}" ;;
	*) var=RV_INJECT val=$n ;;
	esac
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
	# the line of its golden, for -update: a place where the register loop
	# threw that the count of its old instruction names
	[ -z "$update" ] || [ -z "$place" ] || [ ! -s "$key" ] ||
		echo "$place $n $got" > "$work/$name.gline"
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
	why=$(sed -n "s|^$base ||p" "$golden/skip" 2>/dev/null | head -1)
	if [ -n "$why" ]; then
		echo "SKIP $sc: $why"
		continue
	fi
	if [ ! -f "$golden/$base.txt" ] && [ -z "$update" ]; then
		echo "DIFF $sc: no golden $golden/$base.txt (-update writes it from the run, read it first)"
		echo "inject: $sc: 1 differ, it has none"
		status=1
		continue
	fi
	key=$work/$base.count
	rm -f "$key"
	one_run "$work/$base.count.d" "$key" RV_INJECT=2000000000
	# with no injection the run must end as a script does
	c=$(find "$work/$base.count.d" -name "*.txt" | head -1)
	if [ -z "$c" ] || ! grep -q '^--- status exit [01]$' "$c"; then
		echo "DIFF $sc: the run with no injection ends with $( [ -n "$c" ] && sed -n 's/^--- status //p' "$c" || grep '^NOUPDATE' "$work/$base.count.d.log" | cut -c1-200)"
		status=1
	fi
	rm -rf "$work/$base.count.d" "$work/$base.count.d.log"
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
	# each injection recorded too, so that its place is compared even when
	# the sampling here picks others
	if [ -f "$golden/$base.txt" ]; then
		ns=$(printf '%s\n' $ns $(cut -d' ' -f2 "$golden/$base.txt") | LC_ALL=C sort -u)
	fi
	runs=$(echo $ns | wc -w | tr -d ' ')
	hits=$work/$base.hits
	rm -f "$hits"
	out=$work/$base.diffs
	find "$work" -name "$base-*.gline" -exec rm -f {} +
	# shellcheck disable=SC2086
	printf '%s\n' $ns | xargs -P "$jobs" -I{} "$self" -golden "$golden" ${update:+-update} -one "$dir" {} "$sc" > "$out" || true
	if [ -n "$update" ]; then
		find "$work" -name "$base-*.gline" -exec cat {} + 2>/dev/null | LC_ALL=C sort -u > "$golden/$base.txt"
		find "$work" -name "$base-*.gline" -exec rm -f {} +
		echo "UPDATE $golden/$base.txt: $(wc -l < "$golden/$base.txt" | tr -d ' ') places"
	fi
	# the places the script says a run must throw at: "// inject-hit: NAME P"
	sed -n "s|^// inject-hit: \(.*\)$|hit \1|p" "$sc" | while read -r want; do
		grep -qxF "$want" "$hits" 2>/dev/null ||
			echo "DIFF $sc: no run threw at \"${want#hit }\""
	done >> "$out"
	rm -f "$hits"
	bad=$(wc -l < "$out" | tr -d ' ')
	cat "$out"
	echo "inject: $sc: $total places to throw run at $sites sites, $runs injected, $bad differ"
	[ "$bad" = 0 ] || status=1
done
exit $status
