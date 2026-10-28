#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# check.sh fast|full: the checks of a change to the C engine, one table at the
# end (make check-fast, make check-full). The list is below. A change to it
# goes together with docs/testing.md.
#
# Needs the engine generated (make gen) and LLVM 22.1.8 for the generator tests.
#
# Environment:
#   CHECK_ONLY   space separated step names: run only these
#   CHECK_SKIP   space separated step names: leave these out
#   CHECK_LOGS   folder of the logs, one file per step (default .build/check-logs)
#   CHECK_STOP   1: stop at the first failed step (default: go on)
#   LLVM_BIN     bin folder of LLVM 22.1.8 (default /usr/lib/llvm-22/bin)
#   LLD          linker of the generator tests (default: ld.lld or ld, first found)
#   CHECK_LIST   1: print the steps and stop
set -u

tier=${1:-}
case $tier in fast|full) ;; *) echo "usage: check.sh fast|full" >&2; exit 2 ;; esac
cd "$(dirname "$0")/.." || exit 2
root=$(pwd)
llvm=${LLVM_BIN:-/usr/lib/llvm-22/bin}
lld=${LLD:-}
if [ -z "$lld" ]; then
	for f in "$llvm/ld.lld" /usr/bin/ld.lld /usr/bin/ld; do [ -x "$f" ] && { lld=$f; break; }; done
fi
# The scripts of engine/test call clang by its name.
PATH=$llvm:$PATH
export PATH
logs=${CHECK_LOGS:-.build/check-logs}
mkdir -p "$logs"
: > "$logs/summary.tmp"
failed=0

wanted() { # wanted NAME TIER: is the step in this run?
	case " ${CHECK_SKIP:-} " in *" $1 "*) return 1 ;; esac
	if [ -n "${CHECK_ONLY:-}" ]; then
		case " $CHECK_ONLY " in *" $1 "*) return 0 ;; esac
		return 1
	fi
	[ "$2" = fast ] || [ "$tier" = full ]
}

# step NAME TIER SHOW CMD: SHOW is "last" to show the last line of the output
# next to the result, "tests" to show the counts of go test -v, "-" for nothing.
step() {
	name=$1 stier=$2 show=$3 cmd=$4
	if [ "${CHECK_LIST:-}" = 1 ]; then printf '%-14s %s\n' "$name" "$stier"; return; fi
	wanted "$name" "$stier" || return 0
	if [ "$failed" = 1 ] && [ "${CHECK_STOP:-}" = 1 ]; then return 0; fi
	t0=$(date +%s)
	sh -c "$cmd" > "$logs/$name.log" 2>&1
	rc=$?
	t=$(( $(date +%s) - t0 ))
	note=
	case $show in
	last) note=$(grep -v '^[[:space:]]*$' "$logs/$name.log" | tail -n 1 | cut -c1-100) ;;
	tests)
		p=$(grep -c '^[[:space:]]*--- PASS' "$logs/$name.log"); s=$(grep -c '^[[:space:]]*--- SKIP' "$logs/$name.log")
		f=$(grep -c '^[[:space:]]*--- FAIL' "$logs/$name.log")
		note="$p pass, $s skip, $f fail"
		[ "$s" -gt 0 ] && note="$note (skipped: $(grep '^[[:space:]]*--- SKIP' "$logs/$name.log" | sed 's/.*--- SKIP: \([^ ]*\).*/\1/' | head -n 3 | tr '\n' ' '))"
		[ "$f" -gt 0 ] && note="$note (failed: $(grep '^--- FAIL' "$logs/$name.log" | sed 's/--- FAIL: \([^ ]*\).*/\1/' | head -n 3 | tr '\n' ' '))" ;;
	esac
	if [ $rc -eq 0 ]; then res=ok; else res=FAIL; failed=1; [ -z "$note" ] && note=$(tail -n 1 "$logs/$name.log" | cut -c1-100); fi
	printf '%-14s %s (%ss)\n' "$name" "$res" "$t"
	printf '%s\t%s\t%ss\t%s\n' "$name" "$res" "$t" "$note" >> "$logs/summary.tmp"
}

gc="env CGO_ENABLED=0 go"
tags=slowjs_cengine
# Fast: every commit.
step build fast - "$gc build ./..."
step gofmt fast - "out=\$(gofmt -l -s . | grep -v -e ccgo_ -e '^\\.build/' -e '^lib/goasm_' -e '^internal/cengine/engine_'); [ -z \"\$out\" ] || { echo \"\$out\"; exit 1; }"
step docs fast - "scripts/docs-list.sh --check"
step goasm fast tests "env CGO_ENABLED=0 SLOWJS_LLVM=$llvm SLOWJS_SHADOW_PASS=$root/.build/shadow.so SLOWJS_LLD=$lld go test -count=1 -v -p 2 -timeout 60m ./internal/goasm/..."
step cengine fast tests "$gc test -count=1 -v -tags $tags,testhooks ./internal/cengine"
step host-plain fast - "engine/test/build.sh plain"
step compare-plain fast last "engine/test/compare.sh plain"
step cycles fast last "engine/test/cycles.sh"
# Full: the end of a batch.
step fixlib full - "$gc run ./internal/fixlib -check"
step gen full last "scripts/check-gen.sh"
step testhooks full tests "$gc test -count=1 -v -tags $tags,testhooks ./internal/gohost ./internal/hostmem ./internal/idtable"
step root full tests "$gc test -count=1 -v -p 1 -timeout 60m -tags $tags ."
step race full tests "env CGO_ENABLED=1 go test -count=1 -v -race -short -timeout 60m -p 1 -tags $tags . ./internal/gohost ./internal/hostmem ./internal/idtable"
step host-asan full - "engine/test/build.sh asan"
step compare-asan full last "engine/test/compare.sh asan"
step host-ubsan full - "engine/test/build.sh ubsan"
step compare-ubsan full last "engine/test/compare.sh ubsan"
step test262 full last "engine/test/test262.sh -m plain -T 2"

[ "${CHECK_LIST:-}" = 1 ] && exit 0
echo
printf '%-14s %-6s %-7s %s\n' step result time note
while IFS='	' read -r name res t note; do printf '%-14s %-6s %-7s %s\n' "$name" "$res" "$t" "$note"; done < "$logs/summary.tmp"
rm -f "$logs/summary.tmp"
[ "$failed" = 0 ] || { echo "some checks failed: logs in $logs"; exit 1; }
