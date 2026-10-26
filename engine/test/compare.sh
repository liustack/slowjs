#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Compare the C tree with the Go engine on scripts: the transcripts of
# lib/testdata and testdata, and the cases of engine/test/compare. Both run
# each script the same way (engine/test/jsrun.c and internal/jsrun) and what
# they print has to be the same, byte for byte.
#
#   engine/test/compare.sh [MODE]
#
# MODE is a mode of build.sh (default plain), and the host must be built
# already: engine/test/build.sh MODE. The Go runner is built here. Results are in
# engine/test/out/compare-MODE/ (native.txt, go.txt and their diff). Wrap the
# script in taskset or systemd-run to keep it to the CPUs and the memory you
# give it.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(dirname "$(dirname "$here")")
mode=${1:-plain}
out=$here/out/compare-$mode

bin=$here/out/$mode/jsrun
[ -x "$bin" ] || { echo "compare.sh: $bin is missing: run engine/test/build.sh $mode" >&2; exit 2; }
mkdir -p "$out"
(cd "$root" && go build -o "$out/jsrun-go" ./internal/jsrun)

# shikire.js runs on the regexps and the text that its test sets first.
python3 - "$root/lib/testdata" "$out/shikire.js" <<'PY'
import json, sys
d, dst = sys.argv[1], sys.argv[2]
regexps = open(d + '/shiki/regexps.json', encoding='utf-8').read()
text = open(d + '/shiki/input.md', encoding='utf-8').read()
src = open(d + '/shikire.js', encoding='utf-8').read()
open(dst, 'w', encoding='utf-8').write('var shikiRegexps = ' + regexps + ';\nvar shikiInput = ' + json.dumps(text) + ';\n' + src)
PY

t=$root/lib/testdata
files="$t/bitwise.js $t/coldpath.js $t/eq.js $t/forin.js $t/funcshape.js $t/jsonstr.js $t/jsonstr_errors.js $t/objlit.js $t/regex.js $t/regexcache.js $t/regexpv.js $t/regexsplit.js $out/shikire.js"
files="$files $root/testdata/atom_keys.js $root/testdata/field_chain.js $root/testdata/own_keys.js"
files="$files $(ls "$here"/compare/*.js)"
# shellcheck disable=SC2086
"$bin" $files > "$out/native.txt"
# shellcheck disable=SC2086
"$out/jsrun-go" $files > "$out/go.txt"
# What the scripts cannot see, checked in C.
"$here/out/$mode/unit"
if cmp -s "$out/native.txt" "$out/go.txt"; then
	echo "compare: the C tree and the Go engine print the same for $(grep -c '^== ' "$out/go.txt") scripts"
else
	diff "$out/go.txt" "$out/native.txt" > "$out/diff.txt" || true
	echo "compare: the C tree and the Go engine differ, see $out/diff.txt" >&2
	head -40 "$out/diff.txt" >&2
	exit 1
fi
