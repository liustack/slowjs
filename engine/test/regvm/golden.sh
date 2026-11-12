#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# The goldens of the old loop, for when there is one loop only: what each
# test gave with the old loop, recorded while both loops exist, and a run of
# the register loop against it.
#
#   engine/test/regvm/golden.sh [-record | -update] [-inject] [-j N] [-dir G] [MODE]
#
# Without -record it checks: corpus.sh -golden G/corpus (every case of the
# corpus and the unit programs), rvgen check of G/seeds/MODE.txt (1,000 fixed
# seeds of each mode of the generator, frozen in internal/rvgen/v1) and, with
# -inject, inject.sh -max 200 -golden G/inject on engine/test/regvm/js and
# corpus.
# -record writes G again from the old loop, where the two loops agree (a case
# where they differ is listed as NORECORD and left out). -update writes a
# golden that differs from the run again from the run (corpus only): read
# the diff before it goes into a commit.
#
# G is engine/test/regvm/golden by default. MODE is a mode of build.sh
# (default plain), built already: engine/test/regvm/build.sh MODE. -inject
# also needs engine/test/out/inject-MODE, built with EXTRA_DEFS=-DRV_INJECT
# (see inject.sh). RVGEN names a built rvgen for a machine with no Go. It
# exits with 1 when anything differs.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
test=$(dirname "$here")
root=$(dirname "$(dirname "$test")")
g=$here/golden
record=
update=
inject=
jobs=2
seeds=1000
while [ $# -gt 0 ]; do
	case $1 in
	-record) record=1; shift ;;
	-update) update=1; shift ;;
	-inject) inject=1; shift ;;
	-j) jobs=$2; shift 2 ;;
	-dir) g=$2; shift 2 ;;
	*) break ;;
	esac
done
mode=${1:-plain}
if [ -n "$record" ] && [ -n "$update" ]; then
	echo "usage: golden.sh [-record | -update] [-inject] [-j N] [-dir G] [MODE]" >&2
	exit 2
fi
bin=$test/out/$mode/rvdiff
[ -x "$bin" ] || { echo "golden.sh: $bin is missing: run engine/test/regvm/build.sh $mode" >&2; exit 2; }
# a build of the tree now (see engine/test/stamp.sh): inject.sh checks its own
. "$test/stamp.sh"
stamp_check "$(dirname "$test")" "$test/out/$mode" || exit 2
# a built rvgen when RVGEN names one
rvgen=${RVGEN:-$test/out/rvgen}
[ -n "${RVGEN:-}" ] || (cd "$root" && go build -o "$rvgen" ./internal/rvgen/cmd/rvgen)
mkdir -p "$g"
g=$(cd "$g" && pwd)

status=0
if [ -n "$record" ]; then
	rm -rf "$g/corpus" "$g/seeds"
	"$here/corpus.sh" -j "$jobs" -record "$g/corpus" "$mode" || status=1
	mkdir -p "$g/seeds"
	for m in core full wide; do
		f=
		[ $m = core ] && f=-core
		[ $m = wide ] && f=-wide
		"$rvgen" record -rvdiff "$bin" $f -seed 1 -n $seeds -j "$jobs" "$g/seeds/$m.txt" || status=1
	done
	if [ -n "$inject" ]; then
		rm -rf "$g/inject"
		"$here/inject.sh" -j "$jobs" -max 200 -record "$g/inject" "$test/out/inject-$mode" "$here/js" "$here/corpus" || status=1
	fi
	exit $status
fi

"$here/corpus.sh" -j "$jobs" -golden "$g/corpus" ${update:+-update} "$mode" || status=1
for m in core full wide; do
	"$rvgen" check -rvdiff "$bin" -j "$jobs" "$g/seeds/$m.txt" || status=1
done
if [ -n "$inject" ]; then
	"$here/inject.sh" -j "$jobs" -max 200 -golden "$g/inject" "$test/out/inject-$mode" "$here/js" "$here/corpus" || status=1
fi
exit $status
