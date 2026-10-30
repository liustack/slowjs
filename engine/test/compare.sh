#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Compare the C tree with the files kept in the repository: the transcripts
# of engine/test/compare/recorded and engine/test/compare/upstream
# (NAME.txt next to NAME.js), of testdata (NAME.golden), and the cases of
# engine/test/compare (NAME.txt next to NAME.js or NAME.mjs). jsrun
# (engine/test/jsrun.c) runs each script in a fresh runtime, then its jobs,
# and prints its completion value (or what it returns when it is a function),
# and what it prints has to be the file, byte for byte. The files of
# recorded, upstream and testdata hold a lone surrogate as the engine converts
# it (jsrun -raw). The files of engine/test/compare write it as \u{...}.
#
#   engine/test/compare.sh [MODE]
#
# MODE is a mode of build.sh (default plain), and the host must be built
# already: engine/test/build.sh MODE. Results are in
# engine/test/out/compare-MODE/ (NAME.out for each script, and its diff with
# the file when they differ). Wrap the script in taskset or systemd-run to
# keep it to the CPUs and the memory you give it.
#
# COMPARE_UPDATE=native writes the file of each script that has none yet
# (those of engine/test/compare) from the output of the C tree. Read what it
# wrote before it goes into a commit: a file kept is a claim about what the
# engine must print.
#
# A file named FILE#go is the part of FILE before its line "== go: eval": the
# goldens of testdata/error_stack.js hold the stacks the script collects and,
# after them, the stacks the Go API reads, which only a Go host can make.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(dirname "$(dirname "$here")")
mode=${1:-plain}
out=$here/out/compare-$mode
update=${COMPARE_UPDATE:-}

bin=$here/out/$mode/jsrun
[ -x "$bin" ] || { echo "compare.sh: $bin is missing: run engine/test/build.sh $mode" >&2; exit 2; }
mkdir -p "$out"

# shikire.js runs on the regexps and the text that its test sets first.
r="$here/compare/recorded"
{
	printf 'var shikiRegexps = '
	cat "$r/shiki/regexps.json"
	printf ';\nvar shikiInput = "'
	# The text as one JavaScript string: it is ASCII, with no control character
	# but tab and newline.
	sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e 's/	/\\t/g' "$r/shiki/input.md" | awk '{ printf "%s\\n", $0 }'
	printf '";\n'
	cat "$r/shikire.js"
} > "$out/shikire.js"

# test_builtin.js calls std.gc and os.setTimeout of qjs, which the C tree has
# no module for: gc() is the global of jsrun, and a timer is a callback that
# runs once the jobs queued so far have. The first line keeps the "use strict"
# of the file a directive.
{
	printf '"use strict"; var std = { gc: gc };\n'
	printf 'var os = { setTimeout(f) { Promise.resolve().then(() => {}).then(() => {}).then(f); } };\n'
	cat "$here/compare/upstream/test_builtin.js"
} > "$out/test_builtin.js"

t=$here/compare/recorded
# The scripts that have a file in the repository: SCRIPT FILE [OPTIONS of jsrun]
# regexcache.js runs with the cache at its default, at a few entries and off,
# and prints the same each time. fused_ops.js and typeof_is.js run again with
# the instructions the compiler joins into one opcode left apart, and print
# the same.
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
$root/testdata/error_stack.js $root/testdata/error_stack.golden#go -raw -result
$root/testdata/error_stack.js $root/testdata/error_stack_call.golden#go -raw -call
$here/compare/upstream/test_bigint.js $here/compare/upstream/test_bigint.txt -raw
$here/compare/upstream/test_closure.js $here/compare/upstream/test_closure.txt -raw
$here/compare/upstream/test_language.js $here/compare/upstream/test_language.txt -raw
$here/compare/upstream/test_loop.js $here/compare/upstream/test_loop.txt -raw
$here/compare/fused_ops.js $here/compare/fused_ops.txt -no-fused-ops
$here/compare/typeof_is.js $here/compare/typeof_is.txt -no-fused-ops
$out/test_builtin.js $here/compare/upstream/test_builtin.txt -raw
"
# A script of engine/test/compare names the options it needs in a line
# "// jsrun: OPTIONS".
for f in "$here"/compare/*.js "$here"/compare/*.mjs; do
	[ -f "$f" ] || continue
	o=$(sed -n 's,^// jsrun: ,,p' "$f" | head -n 1)
	cases="$cases
$f ${f%.*}.txt $o"
done

total=0
failed=0
printf '%s\n' "$cases" | grep -v '^$' > "$out/cases.list"

while read -r script want opts; do
	# A name for the outputs: the file it must equal, and the cache setting.
	name=$(basename "${want%%#*}")
	tag=${name%.*}
	case $opts in *regexp-cache=4*) tag=$tag-few ;; *regexp-cache=0*) tag=$tag-off ;; esac
	case $opts in *no-fused-ops*) tag=$tag-apart ;; esac
	total=$((total + 1))
	# "== NAME" is the line jsrun puts before each script.
	# shellcheck disable=SC2086
	"$bin" $opts "$script" | tail -n +2 > "$out/$tag.out"
	file=${want%%#*}
	if [ "$file" != "$want" ]; then
		# FILE#go: the part before the stacks of the Go API
		sed '/^== go: eval$/,$d' "$file" > "$out/$tag.want"
		want=$out/$tag.want
	fi
	if [ ! -f "$want" ]; then
		if [ "$update" = native ]; then
			cp "$out/$tag.out" "$want"
			echo "compare: wrote $want from the C tree"
		else
			echo "compare: $want is missing (COMPARE_UPDATE=native writes it)" >&2
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
done < "$out/cases.list"

# What the scripts cannot see, checked in C.
for u in unit unit-oom unit-api unit-white; do
	"$here/out/$mode/$u"
done
if [ "$failed" -ne 0 ]; then
	echo "compare: $failed of $total runs differ" >&2
	exit 1
fi
echo "compare: the C tree prints the files, for $total runs"
