#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Compare the C tree with the files kept in the repository: the transcripts
# of lib/testdata (NAME.txt) and testdata (NAME.golden), and the cases of
# engine/test/compare (NAME.txt next to NAME.js or NAME.mjs). jsrun
# (engine/test/jsrun.c) runs each script in a fresh runtime, then its jobs,
# and prints its completion value (or what it returns when it is a function),
# and what it prints has to be the file, byte for byte. The files of
# lib/testdata and testdata hold a lone surrogate as the engine converts it
# (jsrun -raw). The files of engine/test/compare write it as \u{...}.
#
#   engine/test/compare.sh [MODE]
#
# MODE is a mode of build.sh (default plain), and the host must be built
# already: engine/test/build.sh MODE. Results are in
# engine/test/out/compare-MODE/ (NAME.out for each script, and its diff with
# the file when they differ). Wrap the script in taskset or systemd-run to
# keep it to the CPUs and the memory you give it.
#
# COMPARE_GO=1 also runs the Go engine (internal/jsrun) on every script and
# requires it to print what the C tree prints, in the form that writes a lone
# surrogate as \u{...}. It is for as long as the Go engine is in the
# repository, and has no other use.
#
# COMPARE_UPDATE=native|go writes the file of each script of
# engine/test/compare that has none yet, from the output of the C tree or of
# the Go engine. Read what it wrote before it goes into a commit: a file
# kept is a claim about what the engine must print.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(dirname "$(dirname "$here")")
mode=${1:-plain}
out=$here/out/compare-$mode
update=${COMPARE_UPDATE:-}
withgo=${COMPARE_GO:-}

bin=$here/out/$mode/jsrun
[ -x "$bin" ] || { echo "compare.sh: $bin is missing: run engine/test/build.sh $mode" >&2; exit 2; }
mkdir -p "$out"
if [ -n "$withgo" ] || [ "$update" = go ]; then
	(cd "$root" && go build -o "$out/jsrun-go" ./internal/jsrun)
fi

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
# The scripts that have a file in the repository: SCRIPT FILE [OPTIONS of jsrun]
# regexcache.js runs with the cache at its default, at a few entries and off,
# and prints the same each time.
cases="
$t/bitwise.js $t/bitwise.txt -raw
$t/coldpath.js $t/coldpath.txt -raw
$t/eq.js $t/eq.txt -raw
$t/forin.js $t/forin.txt -raw
$t/funcshape.js $t/funcshape.txt -raw
$t/jsonstr.js $t/jsonstr.txt -raw
$t/jsonstr_errors.js $t/jsonstr_errors.txt -raw
$t/objlit.js $t/objlit.txt -raw
$t/regex.js $t/regex.txt -raw
$t/regexcache.js $t/regexcache.txt -raw
$t/regexcache.js $t/regexcache.txt -raw -regexp-cache=4,1048576
$t/regexcache.js $t/regexcache.txt -raw -regexp-cache=0,0
$t/regexpv.js $t/regexpv.txt -raw
$t/regexsplit.js $t/regexsplit.txt -raw
$out/shikire.js $t/shikire.txt -raw
$root/testdata/atom_keys.js $root/testdata/atom_keys.golden -raw
$root/testdata/field_chain.js $root/testdata/field_chain.golden -raw
$root/testdata/own_keys.js $root/testdata/own_keys.golden -raw
"
for f in "$here"/compare/*.js "$here"/compare/*.mjs; do
	[ -f "$f" ] || continue
	cases="$cases
$f ${f%.*}.txt"
done

total=0
failed=0
printf '%s\n' "$cases" | grep -v '^$' > "$out/cases.list"

while read -r script want opts; do
	name=$(basename "${script%.*}")
	tag=$name
	case $opts in *regexp-cache=4*) tag=$name-few ;; *regexp-cache=0*) tag=$name-off ;; esac
	total=$((total + 1))
	# "== NAME" is the line jsrun puts before each script.
	# shellcheck disable=SC2086
	"$bin" $opts "$script" | tail -n +2 > "$out/$tag.out"
	if [ ! -f "$want" ]; then
		if [ "$update" = native ]; then
			cp "$out/$tag.out" "$want"
			echo "compare: wrote $want from the C tree"
		elif [ "$update" = go ]; then
			"$out/jsrun-go" $opts "$script" | tail -n +2 > "$want"
			echo "compare: wrote $want from the Go engine"
		else
			echo "compare: $want is missing (COMPARE_UPDATE=native or go writes it)" >&2
			failed=$((failed + 1))
		fi
		continue
	fi
	if ! cmp -s "$out/$tag.out" "$want"; then
		diff "$want" "$out/$tag.out" > "$out/$tag.diff" || true
		echo "compare: $tag differs from $want, see $out/$tag.diff" >&2
		head -20 "$out/$tag.diff" >&2
		failed=$((failed + 1))
	fi
	if [ -n "$withgo" ]; then
		# Both in the form that writes a lone surrogate as \u{...}.
		goopts=$(echo "$opts" | sed 's/-raw//')
		# shellcheck disable=SC2086
		"$bin" $goopts "$script" > "$out/$tag.native"
		# shellcheck disable=SC2086
		"$out/jsrun-go" $goopts "$script" > "$out/$tag.go"
		if ! cmp -s "$out/$tag.native" "$out/$tag.go"; then
			diff "$out/$tag.go" "$out/$tag.native" > "$out/$tag.godiff" || true
			echo "compare: $tag differs from the Go engine, see $out/$tag.godiff" >&2
			head -20 "$out/$tag.godiff" >&2
			failed=$((failed + 1))
		fi
	fi
done < "$out/cases.list"

# What the scripts cannot see, checked in C.
"$here/out/$mode/unit"
if [ "$failed" -ne 0 ]; then
	echo "compare: $failed of $total runs differ" >&2
	exit 1
fi
if [ -n "$withgo" ]; then
	echo "compare: the C tree prints the files and what the Go engine prints, for $total runs"
else
	echo "compare: the C tree prints the files, for $total runs"
fi
