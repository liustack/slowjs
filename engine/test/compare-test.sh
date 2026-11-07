#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Test compare.sh with a stand-in for jsrun: output that equals its file passes,
# output that differs fails, and output that equals its file with an exit
# status other than 0 fails too (jsrun exits with 87 for a leak it finds after
# it has printed everything).
#
#   engine/test/compare-test.sh
set -eu
here=$(cd "$(dirname "$0")" && pwd)
d=$(mktemp -d)
trap 'rm -rf "$d"' EXIT
mkdir "$d/bin"

# The stand-in prints the header line, then the file named by $FAKE_OUT, and
# exits with $FAKE_RC.
cat > "$d/bin/jsrun" <<'EOS'
#!/bin/sh
echo "== fake"
# Like the sanitizer: of options given twice the last one counts.
opt() { printf '%s\n' "${UBSAN_OPTIONS:-}" | tr ':' '\n' | sed -n "s/^$1=//p" | tail -n 1; }
if [ -n "${FAKE_UB:-}" ]; then
	echo "runtime error: fake" >&2
	[ "$(opt halt_on_error)" = 1 ] && exit "$(opt exitcode)"
fi
cat "$FAKE_OUT"
exit "${FAKE_RC:-0}"
EOS
for u in unit unit-oom unit-api unit-white; do printf '#!/bin/sh\nexit 0\n' > "$d/bin/$u"; done
chmod +x "$d/bin/"*
printf 'hello\n' > "$d/want.txt"
printf 'hello\n' > "$d/same.txt"
printf 'bye\n' > "$d/other.txt"
printf '%s %s\n' "$d/script.js" "$d/want.txt" > "$d/cases"
: > "$d/script.js"

# run NAME EXPECTED: compare.sh with FAKE_OUT and FAKE_RC from the environment
run() {
	name=$1 want=$2
	set +e
	COMPARE_BIN_DIR=$d/bin COMPARE_OUT=$d/out COMPARE_CASES=$d/cases "$here/compare.sh" plain > "$d/log" 2>&1
	rc=$?
	set -e
	if [ "$want" = pass ] && [ "$rc" -ne 0 ]; then echo "compare-test: $name: should pass, got $rc"; cat "$d/log"; exit 1; fi
	if [ "$want" = fail ] && [ "$rc" -eq 0 ]; then echo "compare-test: $name: should fail, passed"; cat "$d/log"; exit 1; fi
	echo "compare-test: $name: $want, as expected"
	# a shell that keeps the variables set in front of a function call (POSIX
	# mode) must not hand them to the next case
	unset FAKE_UB UBSAN_OPTIONS
}

FAKE_OUT=$d/same.txt FAKE_RC=0 run "same output, exit 0" pass
FAKE_OUT=$d/other.txt FAKE_RC=0 run "other output, exit 0" fail
FAKE_OUT=$d/same.txt FAKE_RC=87 run "same output, exit 87" fail
FAKE_OUT=$d/same.txt FAKE_RC=139 run "same output, killed by a signal" fail
FAKE_OUT=$d/same.txt FAKE_RC=0 FAKE_UB=1 run "a UBSan report in the run" fail
# options the caller already has must not turn the stop off
FAKE_OUT=$d/same.txt FAKE_RC=0 FAKE_UB=1 UBSAN_OPTIONS=halt_on_error=0 run "a UBSan report, halt_on_error=0 inherited" fail
FAKE_OUT=$d/same.txt FAKE_RC=0 FAKE_UB=1 UBSAN_OPTIONS=halt_on_error=0:exitcode=0 run "a UBSan report, exitcode=0 inherited" fail
FAKE_OUT=$d/same.txt FAKE_RC=0 FAKE_UB=1 UBSAN_OPTIONS=print_stacktrace=0 run "a UBSan report, other options inherited" fail
FAKE_OUT=$d/same.txt FAKE_RC=0 UBSAN_OPTIONS=halt_on_error=0:exitcode=0 run "no report, options inherited" pass

