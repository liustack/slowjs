#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Test the lookup of llvm-bin.sh with stand-ins for clang: LLVM_BIN wins, the
# first folder of the list with a clang is taken, a wrong version or no folder
# stops it, and -f finds without the version check.
#
#   scripts/llvm-bin-test.sh
set -eu
here=$(cd "$(dirname "$0")" && pwd)
d=$(mktemp -d)
trap 'rm -rf "$d"' EXIT

fake() { # fake FOLDER VERSION: a clang that says VERSION
	mkdir -p "$1"
	printf '#!/bin/sh\necho "clang version %s"\n' "$2" > "$1/clang"
	chmod +x "$1/clang"
}
fake "$d/good1" 22.1.8
fake "$d/good2" 22.1.8
fake "$d/old" 21.1.0
mkdir "$d/empty"

run() { # run NAME EXPECT(pass|fail) WANTED-OUTPUT ARGS... (environment as set by the caller)
	name=$1 expect=$2 wantout=$3
	shift 3
	set +e
	"$here/llvm-bin.sh" "$@" > "$d/out" 2> "$d/err"
	rc=$?
	set -e
	if [ "$expect" = pass ] && [ "$rc" -ne 0 ]; then echo "llvm-bin-test: $name: should pass, got $rc"; cat "$d/err"; exit 1; fi
	if [ "$expect" = fail ] && [ "$rc" -eq 0 ]; then echo "llvm-bin-test: $name: should fail, printed:"; cat "$d/out"; exit 1; fi
	[ "$(cat "$d/out")" = "$wantout" ] || { echo "llvm-bin-test: $name: printed '$(cat "$d/out")', want '$wantout'"; exit 1; }
	echo "llvm-bin-test: $name: $expect, as expected"
}

export LLVM_BIN_CANDIDATES="$d/empty $d/good1 $d/good2"
unset LLVM_BIN
run "the first folder with a clang" pass "$d/good1"
( export LLVM_BIN=$d/good2; run "LLVM_BIN wins" pass "$d/good2" )
( export LLVM_BIN=$d/old; run "LLVM_BIN with another version" fail "" )
grep -q "21.1.0" "$d/err" || { echo "llvm-bin-test: the error does not name the version found"; cat "$d/err"; exit 1; }
( export LLVM_BIN=$d/old; run "LLVM_BIN with another version, -f" pass "$d/old" -f )
( export LLVM_BIN=$d/empty; run "LLVM_BIN with no clang" fail "" )
( export LLVM_BIN=$d/empty; run "LLVM_BIN with no clang, -f" pass "$d/empty" -f )
( export LLVM_BIN_CANDIDATES="$d/old"; run "the only folder is the wrong version" fail "" )
( export LLVM_BIN_CANDIDATES="$d/empty"; run "no folder" fail "" )
grep -q "brew install llvm@22" "$d/err" && grep -q "apt.llvm.org" "$d/err" && grep -q "LLVM_BIN" "$d/err" || { echo "llvm-bin-test: the error does not say what to install"; cat "$d/err"; exit 1; }
( export LLVM_BIN_CANDIDATES="$d/empty"; run "no folder, -f" fail "" -f )
