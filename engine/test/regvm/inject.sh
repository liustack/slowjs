#!/bin/sh
# Copyright (c) 2026 The liustack/slowjs Authors.
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
# place the goldens have is compared with its golden, first recorded from
# the old loop's run with the same throw at deb70815: the status, and the
# count and checksum of each kind (output, result, exception, stack, job).
# A run at a place with no golden, or with no place, is judged by its
# status: it must end with exit 0 or 1 (an uncaught throw). A leak (87: the
# host found blocks still in use when the runtime was freed, or rvdiff found
# the runtime holds other counts of objects, atoms or shapes once the
# context is freed than before it was made), a sanitizer report (86), a
# signal (an abort of a self check, as a value freed twice gives), the time
# limit, an instruction that was to throw and ran to its end ("miss"), or
# whose throw a hook of JS it ran took ("outside"), is a difference. The
# run with no injection must end with exit 0 or 1. The counts of memory at
# the end of a run are not in the goldens: they move with any change of what
# the engine keeps, and the leak check above compares the run with itself.
#
# A run that throws also says at which hook: "hit NAME P", the P-th place
# of the register instruction NAME. A script that has a line
# "// inject-hit: NAME P" fails when no run threw there, so that a place a
# test is written for cannot be lost. A place of G/SCRIPT.txt that the
# runs were to compare and that no run threw at differs too: the places of
# the script moved, and its golden is to be written again.
#
#   engine/test/regvm/inject.sh [-j N] [-max M] [-golden G] [-update] DIR PATH...
#   engine/test/regvm/inject.sh [-j N] [-every K] [-golden G] DIR PATH...
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
# and runs nothing. The places hold while the bytecode of the script is the
# same. It prints one line per run that differs and a line for each script,
# and exits with 1 when any run differs. The transcripts of a run that
# differs are in DIR/inject/SCRIPT-N/.
#
# -update writes G/SCRIPT.txt again from the run, for a new script or a
# change of behaviour that is meant (make inject-goldens does it for the
# whole set). For each place it prints the keys that changed ("CHANGED
# SCRIPT PLACE: KEY,..."), the places no run reaches any more ("GONE") and
# those that are new ("NEW"), then the counts of each script and of all. A
# run that fails by its status (a leak, a signal) writes no line: the old
# line of its place stays, and the run still differs. Read the diff before
# it goes into a commit.
#
# -every K runs none of the places the sampling picks: of each script only the
# lines 1, 1 + K, 1 + 2K and so on of G/SCRIPT.txt and its last line, besides
# the run with no injection, and it reads no "inject-hit" line (the run that
# throws there may not be among them). The choice is the same on every run of
# the same goldens, so that a commit always gets the same verdict: a golden
# that a change moved, and that was not written again, differs at the lines
# picked, unless the change moved none of them. The whole set checks them all.
# The last line gives the count of scripts, runs and runs that differ.
#
# -goldens builds and runs nothing: it names each script of PATH with neither
# a G/SCRIPT.txt nor a line in G/skip, and exits with 1 when there is one, so
# that a script added without its golden shows before the injections run.
set -eu

self=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")
jobs=4
max=2000
every=
single=
update=
goldens=
golden=$(cd "$(dirname "$0")" && pwd)/golden/inject
while [ $# -gt 0 ]; do
	case $1 in
	-j) jobs=$2; shift 2 ;;
	-max) max=$2; shift 2 ;;
	-every) every=$2; shift 2 ;;
	-golden) golden=$2; shift 2 ;;
	-update) update=1; shift ;;
	-goldens) goldens=1; shift ;;
	-one) single=1; shift ;; # -one DIR N SCRIPT: one run, for xargs
	-*) echo "usage: inject.sh [-j N] [-max M] [-every K] [-golden G] [-update] DIR PATH..., or inject.sh [-golden G] -goldens PATH..." >&2; exit 2 ;;
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
	echo "usage: inject.sh [-j N] [-max M] [-every K] [-golden G] [-update] DIR PATH..., or inject.sh [-golden G] -goldens PATH..." >&2
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
# count and checksum of each kind (the hash of a kind kept as one) but expect
# and the counts of memory, mem-end and mem-gc. Those move with any change
# of what the engine keeps (a shape held for the context, an object it no
# longer makes) and say nothing of the throw: a leak is the status of the
# run, 87, which rvdiff gives when the runtime holds more once the context
# is freed than before it was made.
summary() {
	s="status=$(sed -n 's/^--- status //p' "$1" | tr ' ' _)"
	for k in out result exception stack job job-stack; do
		h=$(grep "^hash $k " "$1" | cut -d' ' -f3,4 | tr ' ' /)
		if [ -n "$h" ]; then
			s="$s $k=fnv$h"
		elif grep -q "^$k " "$1"; then
			s="$s $k=$(grep "^$k " "$1" | cksum | cut -d' ' -f1)/$(grep -c "^$k " "$1")"
		fi
	done
	echo "$s"
}

# changes OLD NEW KEYS: for -update, a line for each place of the golden OLD
# that the golden NEW gives other values ("CHANGED SCRIPT PLACE: KEY,..."),
# has and OLD has not ("NEW") or has not and OLD has ("GONE"). It writes to
# KEYS the count of places of each key that changed and of the new and gone
# ones, a line "NAME COUNT" each.
changes() {
	awk -v sc="$sc" -v keys="$3" -v oldf="$1" '
	function fields(line, m,   i, n, f, k) {
		split("", m)
		n = split(line, f, " ")
		for (i = 3; i <= n; i++) {
			k = f[i]
			sub(/=.*/, "", k)
			m[k] = f[i]
		}
	}
	FILENAME == oldf { old[$1] = $0; next }
	{ new[$1] = $0 }
	END {
		for (p in new)
			if (!(p in old)) {
				print "NEW " sc " " p
				count["new"]++
			}
		for (p in old) {
			if (!(p in new)) {
				print "GONE " sc " " p
				count["gone"]++
				continue
			}
			fields(old[p], a)
			fields(new[p], b)
			changed = ""
			for (k in b)
				if (!(k in a)) a[k] = ""
			for (k in a)
				if (a[k] != b[k]) {
					changed = changed "," k
					count["key " k]++
				}
			if (changed != "") {
				print "CHANGED " sc " " p ": " substr(changed, 2)
				count["changed"]++
			}
		}
		for (k in count)
			print k, count[k] > keys
		close(keys)
	}' "$1" "$2"
	[ -f "$3" ] || : > "$3"
}

# changes_line KEYS: the counts of changes of KEYS (lines "NAME COUNT", of
# one golden or summed over several) as a line: "C changed (KEY N, ...), A
# new, D gone"
changes_line() {
	awk '
	{ n[$1 == "key" ? "key " $2 : $1] += $NF }
	END {
		order = "status out result exception stack job job-stack"
		m = split(order, o, " ")
		ks = ""
		for (i = 1; i <= m; i++)
			if (("key " o[i]) in n) {
				ks = ks ", " o[i] " " n["key " o[i]]
				delete n["key " o[i]]
			}
		for (k in n)
			if (k ~ /^key /)
				ks = ks ", " substr(k, 5) " " n[k]
		printf "%d changed", n["changed"]
		if (ks != "") printf " (%s)", substr(ks, 3)
		printf ", %d new, %d gone\n", n["new"], n["gone"]
	}' "$1"
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
	# for -update, a run that failed (a leak, a signal, the time limit): it
	# writes no golden line, the old line of its place stays
	keep_line() {
		[ -z "$update" ] || [ -z "$place" ] ||
			grep "^$place " "$golden/$base.txt" > "$work/$name.gline" 2>/dev/null || true
	}
	one_run "$work/$name" "$key" "$var=$val"
	grep "^hit " "$key" >> "$work/$base.hits" 2>/dev/null || true
	at=$(grep -E '^(at|nokey|real|skip|miss|outside)' "$key" 2>/dev/null || echo '?')
	place=$(echo "$at" | sed -n 's/^at //p' | tr ' ' _)
	# the places reached, by a run that differs too: each place of the golden
	# the runs were to compare must be one of them
	[ -z "$place" ] || echo "$place" >> "$work/$base.reached"
	g=$(find "$work/$name" -name "*.txt" | head -1)
	if [ -z "$g" ]; then
		echo "DIFF $sc N=$n status ($at): $(grep '^NOUPDATE' "$work/$name.log" | cut -c1-200)"
		keep_line
		exit 0
	fi
	want=
	[ -z "$place" ] || want=$(grep "^$place " "$golden/$base.txt" 2>/dev/null | cut -d' ' -f3- || true)
	got=$(summary "$g")
	# the line of its golden, for -update: a place where the register loop
	# threw that the count of its old instruction names, in a run that ended
	# as a script does
	if [ -n "$update" ] && [ -n "$place" ] && [ -s "$key" ]; then
		if grep -q '^--- status exit [01]$' "$g"; then
			echo "$place $n $got" > "$work/$name.gline"
		else
			keep_line
		fi
	fi
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

case $every in
'') ;;
*[!0-9]* | 0) echo "inject.sh: -every takes a count of 1 or more" >&2; exit 2 ;;
*) [ -z "$update" ] || { echo "inject.sh: -every with -update would write a golden of the lines picked only" >&2; exit 2; } ;;
esac

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
nscripts=0 nruns=0 nbad=0
for sc in $scripts; do
	base=$(basename "$sc")
	why=$(sed -n "s|^$base ||p" "$golden/skip" 2>/dev/null | head -1)
	if [ -n "$why" ]; then
		echo "SKIP $sc: $why"
		continue
	fi
	nscripts=$((nscripts + 1))
	if [ ! -f "$golden/$base.txt" ] && [ -z "$update" ]; then
		echo "DIFF $sc: no golden $golden/$base.txt (-update writes it from the run, read it first)"
		echo "inject: $sc: 1 differ, it has none"
		status=1
		nbad=$((nbad + 1))
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
		nbad=$((nbad + 1))
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
		nbad=$((nbad + 1))
		continue
	fi
	if [ -n "$every" ]; then
		# the lines picked of the golden, in the order of the file
		ns=$(awk -v k="$every" '(NR - 1) % k == 0 { print $2 } END { print $2 }' "$golden/$base.txt" | LC_ALL=C sort -u)
	elif [ "$total" -le "$max" ]; then
		ns=$(seq 1 "$total")
	else
		each=$((max / sites))
		[ "$each" -ge 1 ] || each=1
		ns=$(awk -v s="$sites" -v q="$each" \
			'BEGIN { for (i = 1; i <= s; i++) for (j = 1; j <= q; j++) print i "." j }')
	fi
	# each injection recorded too, so that its place is compared even when
	# the sampling here picks others
	if [ -z "$every" ] && [ -f "$golden/$base.txt" ]; then
		ns=$(printf '%s\n' $ns $(cut -d' ' -f2 "$golden/$base.txt") | LC_ALL=C sort -u)
	fi
	runs=$(echo $ns | wc -w | tr -d ' ')
	hits=$work/$base.hits
	rm -f "$hits" "$work/$base.reached"
	out=$work/$base.diffs
	find "$work" -maxdepth 1 -name "$base-*.gline" -exec rm -f {} +
	# shellcheck disable=SC2086
	printf '%s\n' $ns | xargs -P "$jobs" -I{} "$self" -golden "$golden" ${update:+-update} -one "$dir" {} "$sc" > "$out" || true
	# for -update, one line a place: two runs can throw at one place when the
	# places of a script moved
	if [ -n "$update" ]; then
		find "$work" -maxdepth 1 -name "$base-*.gline" -exec cat {} + 2>/dev/null | LC_ALL=C sort |
			LC_ALL=C sort -u -k1,1 > "$work/$base.new"
		find "$work" -maxdepth 1 -name "$base-*.gline" -exec rm -f {} +
		if [ -f "$golden/$base.txt" ]; then
			changes "$golden/$base.txt" "$work/$base.new" "$work/$base.keys" | LC_ALL=C sort
			what=$(changes_line "$work/$base.keys")
			cat "$work/$base.keys" >> "$work/update.$$"
			rm -f "$work/$base.keys"
		else
			what="a new golden"
		fi
		mv "$work/$base.new" "$golden/$base.txt"
		echo "UPDATE $golden/$base.txt: $(wc -l < "$golden/$base.txt" | tr -d ' ') places, $what"
	elif [ -f "$golden/$base.txt" ]; then
		# each place of the golden the runs were to compare was reached: a
		# place no run threw at any more is compared with nothing
		if [ -n "$every" ]; then
			awk -v k="$every" '(NR - 1) % k == 0 { print } END { print }' "$golden/$base.txt"
		else
			cat "$golden/$base.txt"
		fi | LC_ALL=C sort -u -k1,1 > "$work/$base.want"
		LC_ALL=C sort -u "$work/$base.reached" 2>/dev/null | LC_ALL=C join -v 1 "$work/$base.want" - |
			while read -r p n rest; do
				echo "DIFF $sc: no run threw at the place $p of the golden (its run $n): the places of the script moved, record its golden again (inject.sh -update)"
			done >> "$out"
		rm -f "$work/$base.want"
	fi
	rm -f "$work/$base.reached"
	# the places the script says a run must throw at: "// inject-hit: NAME P"
	[ -n "$every" ] || sed -n "s|^// inject-hit: \(.*\)$|hit \1|p" "$sc" | while read -r want; do
		grep -qxF "$want" "$hits" 2>/dev/null ||
			echo "DIFF $sc: no run threw at \"${want#hit }\""
	done >> "$out"
	rm -f "$hits"
	bad=$(wc -l < "$out" | tr -d ' ')
	cat "$out"
	echo "inject: $sc: $total places to throw run at $sites sites, $runs injected, $bad differ"
	nruns=$((nruns + runs)) nbad=$((nbad + bad))
	[ "$bad" = 0 ] || status=1
done
if [ -n "$update" ] && [ -f "$work/update.$$" ]; then
	echo "inject: the goldens written again, against the old ones: $(changes_line "$work/update.$$")"
	rm -f "$work/update.$$"
fi
echo "inject: $nscripts scripts, $nruns injected, $nbad differ"
exit $status
