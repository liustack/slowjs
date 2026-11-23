#!/bin/sh
# Copyright (c) 2026 The liustack/tifajs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE at the
# root of this repository.
#
# Test inject-cover.sh with stand-ins for rvdiff and the preprocessor (no
# build): two scripts of one name in two folders each count, a run that
# does not end as a script does (a leak, status 87) fails the script, and a
# preprocessor that fails, or that gives no hook, stops it.
#
#   engine/test/regvm/inject-cover-test.sh
set -eu
here=$(cd "$(dirname "$0")" && pwd)
d=$(mktemp -d)
trap 'rm -rf "$d"' EXIT

fail() {
	echo "inject-cover-test: $1"
	cat "$d/log"
	exit 1
}

# run NAME CODE [VAR=VAL...]: inject-cover.sh on the two scripts with the
# stand-ins, which must exit with CODE
run() {
	name=$1 want=$2
	shift 2
	set +e
	env CC="$d/cc" "$@" "$here/inject-cover.sh" -j 2 -golden "$d/golden" "$d/bin" "$d/a/t.js" "$d/b/t.js" > "$d/log" 2>&1
	rc=$?
	set -e
	[ "$rc" -eq "$want" ] || fail "$name: should exit with $want, got $rc"
	echo "inject-cover-test: $name: exit $want, as expected"
}

# has NAME TEXT: the log of the last run has a line with TEXT
has() {
	grep -qF "$2" "$d/log" || fail "$1: no line with \"$2\""
}

mkdir "$d/bin" "$d/golden" "$d/a" "$d/b"
: > "$d/a/t.js"
: > "$d/b/t.js"
# The stand-in of rvdiff: a script of the folder a takes the hook 1 of the
# preprocessor, one of b the hook 2. The cover runs end with the status
# $FAKE_COVER_STATUS (0 by default).
cat > "$d/bin/rvdiff" <<'EOS'
#!/bin/sh
for a; do sc=$a; done
dir=$3
st=0
echo "none 10 2" >> "$RV_INJECT_OUT"
if [ -n "${RV_INJECT_COVER:-}" ]; then
	id=1
	case $sc in */b/*) id=2 ;; esac
	echo "cover ADD - regvm-a.h:$((4 + id)):$id" >> "$RV_INJECT_OUT"
	st=${FAKE_COVER_STATUS:-0}
fi
printf 'result none\n--- status exit %s\n' "$st" > "$dir/$(basename "$sc").txt"
EOS
chmod +x "$d/bin/rvdiff"
# The stand-in of the preprocessor: the two hooks, then the status
# $FAKE_CC_STATUS (0 by default). With $FAKE_CC_EMPTY it gives no hook.
cat > "$d/cc" <<'EOS'
#!/bin/sh
if [ -z "${FAKE_CC_EMPTY:-}" ]; then
	echo 'x = (rv_inj_hit(ctx, "engine/regvm-a.h", 5, 1));'
	echo 'y = (rv_inj_hit(ctx, "engine/regvm-a.h", 6, 2));'
fi
exit "${FAKE_CC_STATUS:-0}"
EOS
chmod +x "$d/cc"

run "two scripts of one name" 0
has "two scripts of one name" "inject-cover: 2 scripts, 2 hooks, 2 reached, 0 reached by none"

run "a cover run that leaks" 1 FAKE_COVER_STATUS=87
has "a cover run that leaks" "FAIL $d/a/t.js: the run with RV_INJECT_COVER=0 1 ends with exit 87"

run "a preprocessor that fails" 1 FAKE_CC_STATUS=1
! grep -q '^inject-cover: ' "$d/log" || fail "a preprocessor that fails: it still counted"

run "a preprocessor that gives no hook" 2 FAKE_CC_EMPTY=1
has "a preprocessor that gives no hook" "gives no hook"
