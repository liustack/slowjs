#!/bin/sh
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Test inject.sh. With no argument, with a stand-in for rvdiff (no build):
# a golden holds no memory counts, so a run whose counts moved alone passes,
# and a run whose result moved fails.
#
# With DIR, a build with -DRV_INJECT (see inject.sh), it runs that build
# instead: it records the golden of a small script, then plants a leak in
# each run where the injection threw (RV_INJECT_PLANT of rvdiff, an atom and
# an object) and checks that every run fails, at the places of the golden
# and at the places with no key.
#
#   engine/test/regvm/inject-test.sh [DIR]
set -eu
here=$(cd "$(dirname "$0")" && pwd)
engine=$(dirname "$(dirname "$here")")
d=$(mktemp -d)
trap 'rm -rf "$d"' EXIT

fail() {
	echo "inject-test: $1"
	cat "$d/log"
	exit 1
}

# run NAME pass|fail [OPTIONS]: inject.sh on the script with the stand-in
run() {
	name=$1 want=$2
	shift 2
	set +e
	"$here/inject.sh" -j 2 -golden "$d/golden" "$@" "$bin" "$d/s.js" > "$d/log" 2>&1
	rc=$?
	set -e
	if [ "$want" = pass ] && [ "$rc" -ne 0 ]; then fail "$name: should pass, got $rc"; fi
	if [ "$want" = fail ] && [ "$rc" -eq 0 ]; then fail "$name: should fail, passed"; fi
	echo "inject-test: $name: $want, as expected"
}

# has NAME TEXT: the log of the last run has a line with TEXT
has() {
	grep -qF "$2" "$d/log" || fail "$1: no line with \"$2\""
}

if [ $# -eq 0 ]; then
	bin=$d/bin
	mkdir "$bin" "$d/golden"
	# a build of the tree now, as far as inject.sh can tell
	. "$engine/test/stamp.sh"
	printf 'src %s\ntools %s\n' "$(stamp_src "$engine")" "$(stamp_tools "$engine")" > "$d/bin/regvm.stamp"
	# The stand-in takes the places from $FAKE_PLACES, lines "N FUNC POS K
	# RESULT [STATUS]": the N-th injection throws at the place FUNC_POS_K and
	# the script then gives RESULT and exits with STATUS (0 by default). A
	# run with no line throws at no place ("nokey"). The counts of memory are
	# $FAKE_MEM.
	cat > "$d/bin/rvdiff" <<'EOS'
#!/bin/sh
for a; do sc=$a; done
dir=$3
n=${RV_INJECT:-}
[ -z "${RV_INJECT_SITE:-}" ] || n=$(echo "$RV_INJECT_SITE" | tr ' ' .)
if [ "$n" = 2000000000 ]; then
	echo "none $(wc -l < "$FAKE_PLACES" | tr -d ' ') 3" >> "$RV_INJECT_OUT"
	result=none
else
	key=$(awk -v n="$n" '$1 == n { print $2, $3, $4 }' "$FAKE_PLACES")
	result=$(awk -v n="$n" '$1 == n { print $5 }' "$FAKE_PLACES")
	st=$(awk -v n="$n" '$1 == n { print $6 }' "$FAKE_PLACES")
	if [ -n "$key" ]; then
		printf 'at %s\nhit OP 1\n' "$key" >> "$RV_INJECT_OUT"
	else
		echo nokey >> "$RV_INJECT_OUT"
	fi
fi
printf 'result %s\nmem-end obj=%s\nmem-gc obj=%s\n--- status exit %s\n' "$result" "$FAKE_MEM" "$FAKE_MEM" "${st:-0}" > "$dir/$(basename "$sc").txt"
EOS
	chmod +x "$d/bin/rvdiff"
	: > "$d/s.js"
	printf '1 1 1 1 a\n2 1 2 1 b\n3 1 3 1 c\n4 2 1 1 d\n5 2 2 1 e\n' > "$d/places"
	export FAKE_PLACES="$d/places"

	FAKE_MEM=10 run "record the golden" pass -update
	[ "$(wc -l < "$d/golden/s.js.txt" | tr -d ' ')" = 5 ] || fail "record the golden: $(cat "$d/golden/s.js.txt")"
	! grep -q mem- "$d/golden/s.js.txt" || fail "the golden holds counts of memory: $(cat "$d/golden/s.js.txt")"
	FAKE_MEM=10 run "the same run" pass
	FAKE_MEM=11 run "the counts of memory moved alone" pass
	FAKE_MEM=11 run "the counts of memory moved alone, -every 2" pass -every 2

	printf '1 1 1 1 a\n2 1 2 1 B\n3 1 3 1 c\n4 2 1 1 d\n5 2 2 1 e\n' > "$d/places2"
	FAKE_PLACES=$d/places2 FAKE_MEM=10 run "a result moved" fail
	has "a result moved" "DIFF $d/s.js N=2 at 1 2 1"

	exit 0
fi

# A build with -DRV_INJECT: a script with a place that has a key and places
# that have none (the interrupt of a conditional jump), recorded, then run
# with a leak planted in each run that throws.
bin=$(cd "$1" && pwd)
mkdir "$d/golden"
cat > "$d/s.js" <<'EOS'
"use strict";
function f(o, n) {
	let s = 0;
	for (let i = 0; i < n; i++)
		s += o.x + i;
	return s;
}
f({ x: 1 }, 2);
f({ x: 2 }, 3);
EOS
run "record the golden" pass -update
run "the golden recorded" pass
runs=$(sed -n 's/.* \([0-9][0-9]*\) injected, [0-9][0-9]* differ$/\1/p' "$d/log" | head -1)
for what in atom object; do
	RV_INJECT_PLANT=$what run "an $what leaked after each injected throw" fail
	bad=$(sed -n 's/.* injected, \([0-9][0-9]*\) differ$/\1/p' "$d/log" | head -1)
	[ "$bad" = "$runs" ] || fail "an $what leaked after each injected throw: $bad of $runs runs differ"
	grep -q '^DIFF .* N=[0-9.]* status (nokey)' "$d/log" || fail "an $what leaked after each injected throw: no run at a place with no key failed"
done
unset RV_INJECT_PLANT
