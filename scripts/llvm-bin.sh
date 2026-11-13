#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# llvm-bin.sh [-f]: print the bin folder of LLVM 22.1.8, the one place that
# finds it. check.sh, gen-engine.sh, go-versions.sh, engine/test/cycles.sh and
# engine/pgo/train.sh call it. The folder is, in this order:
#   1. LLVM_BIN, when set
#   2. the first of these that has a clang: /usr/lib/llvm-22/bin (apt.llvm.org),
#      /opt/homebrew/opt/llvm@22/bin (Homebrew on Apple silicon) and
#      /usr/local/opt/llvm@22/bin (Homebrew on Intel)
# and its clang must be version 22.1.8, or the script says so and exits 1. With
# no folder it exits 1 and says what to install.
#
# -f only finds: no version check, no message, and it exits 1 when nothing is
# there (go-versions.sh leaves the LLVM steps out then).
#
# Test: scripts/llvm-bin-test.sh. LLVM_BIN_CANDIDATES (folders separated by
# spaces) replaces the three folders of step 2, for that test alone.
set -eu

want=22.1.8
find_only=0
case ${1:-} in
-f) find_only=1 ;;
'') ;;
*) echo "usage: llvm-bin.sh [-f]" >&2; exit 2 ;;
esac

dir=
if [ -n "${LLVM_BIN:-}" ]; then
	dir=$LLVM_BIN
else
	for d in ${LLVM_BIN_CANDIDATES:-/usr/lib/llvm-22/bin /opt/homebrew/opt/llvm@22/bin /usr/local/opt/llvm@22/bin}; do
		if [ -x "$d/clang" ]; then dir=$d; break; fi
	done
fi

if [ "$find_only" = 1 ]; then
	[ -n "$dir" ] || exit 1
	echo "$dir"
	exit 0
fi

if [ -z "$dir" ]; then
	cat >&2 <<EOM
llvm-bin.sh: no LLVM $want found. Install it and run again:
  Linux (Ubuntu, Debian): the packages clang-22, llvm-22-dev and lld-22 from apt.llvm.org (/usr/lib/llvm-22/bin)
  macOS: brew install llvm@22 and brew pin llvm@22 (so brew upgrade keeps it), then check that clang --version in its bin folder says $want (Homebrew: /opt/homebrew/opt/llvm@22/bin)
  Another place: set LLVM_BIN to the bin folder of LLVM $want
EOM
	exit 1
fi
got=$("$dir/clang" --version 2>/dev/null | sed -n 's/.*clang version \([0-9][0-9.]*\).*/\1/p' | head -n 1)
if [ "$got" != "$want" ]; then
	echo "llvm-bin.sh: clang in $dir is '${got:-missing}': the engine is generated with LLVM $want only (LLVM_BIN picks another folder)" >&2
	exit 1
fi
echo "$dir"
