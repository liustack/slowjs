#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Check what the C tree asks of the outside. It builds the engine files and
# the files of engine/libc as the generated assembly will be built (freestanding,
# no system headers), takes the symbols they use and do not define, and fails
# for any that engine/libc/symbols.txt does not allow. Twice:
#
# - the engine files alone may leave libc names, host callbacks and the
#   allocator undefined, and nothing else
# - the engine files with engine/libc may leave host callbacks and the
#   allocator undefined, and nothing else, so engine/libc defines every libc
#   name the engine uses
#
# It does that for each optimization level (the compiler turns some loops and
# struct copies into library calls, and folds others away) and with and without
# NDEBUG.
#
#   engine/test/check-symbols.sh [TARGET...]
#
# A TARGET is a clang target triple, and with none given the machine's own is
# used. CC and NM pick the compiler and the symbol lister. Objects of another
# target are only compiled, so no linker or system headers are needed:
#
#   CC=clang NM=llvm-nm engine/test/check-symbols.sh x86_64-linux-gnu aarch64-linux-gnu
set -eu

here=$(cd "$(dirname "$0")" && pwd)
engine=$(dirname "$here")
libc=$engine/libc
cc=${CC:-clang}
nm=${NM:-nm}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

. "$here/flags.sh"
version=$engine_version
free=$engine_flags

allowed() { sed -n "s/^$1 \(.*\)/\1/p" "$libc/symbols.txt"; }
allowed libc > "$work/libc.txt"
allowed host > "$work/host.txt"
allowed alloc > "$work/alloc.txt"
cat "$work/host.txt" "$work/alloc.txt" > "$work/outside.txt"
cat "$work/libc.txt" "$work/outside.txt" > "$work/all.txt"

# The names the objects define (-g --defined-only) or use (-u), sorted, in the
# file OUT. nm is run on each object on its own, and a failure of nm, or
# anything it says on its error output, ends the script: a symbol table that
# was not read must not look like an empty one. A Mach-O symbol has a leading
# underscore, which is removed. Call it directly, not in a pipeline or a
# command substitution, or the exit ends only that.
list() { # MODE FILE OBJECT...
	list_mode=$1
	list_file=$2
	shift 2
	: > "$list_file.raw"
	for o in "$@"; do
		if ! "$nm" $list_mode "$o" > "$work/nm.out" 2> "$work/nm.err" || [ -s "$work/nm.err" ]; then
			echo "check-symbols: $nm $list_mode $o failed: $(cat "$work/nm.err")" >&2
			exit 1
		fi
		cat "$work/nm.out" >> "$list_file.raw"
	done
	if [ "$list_mode" = -u ]; then
		awk 'NF >= 1 {print $NF}' "$list_file.raw" > "$list_file.names"
	else
		awk 'NF >= 3 {print $3}' "$list_file.raw" > "$list_file.names"
	fi
	if [ "$strip" = 1 ]; then sed 's/^_//' "$list_file.names" > "$list_file.fix"; else cp "$list_file.names" "$list_file.fix"; fi
	sort -u "$list_file.fix" > "$list_file"
}
# What the objects use and none of them defines, in the file FILE.
open() { # FILE OBJECT...
	open_file=$1
	shift
	list -u "$work/u" "$@"
	list "-g --defined-only" "$work/d" "$@"
	comm -23 "$work/u" "$work/d" > "$open_file"
}

fail=0
check() { # description, symbols file of what is open, allowlist file
	sort -u "$3" > "$work/allow.sorted"
	sort -u "$2" | comm -23 - "$work/allow.sorted" > "$work/bad.txt"
	if [ -s "$work/bad.txt" ]; then
		echo "check-symbols: $1: not allowed: $(tr '\n' ' ' < "$work/bad.txt")" >&2
		fail=1
	fi
}

# The check has to reject what it is for: an object that calls a function
# nobody allows, and an object nm cannot read.
cat > "$work/bad.c" <<'BAD'
extern int rand(void);
extern void *host_now_not_listed(void);
int bad(void) { return rand() + (host_now_not_listed() != 0); }
BAD
$cc -c "$work/bad.c" -o "$work/bad.o"
strip=0
[ "$(uname -s)" = Darwin ] && strip=1
open "$work/bad-open.txt" "$work/bad.o"
check "self-test" "$work/bad-open.txt" "$work/all.txt" 2> /dev/null
if [ $fail = 0 ]; then
	echo "check-symbols: the check let an unlisted symbol through" >&2
	exit 1
fi
fail=0
echo "this is not an object" > "$work/garbage.o"
if (open "$work/garbage-open.txt" "$work/garbage.o") 2> /dev/null; then
	echo "check-symbols: the check read an object that is not one" >&2
	exit 1
fi

targets=${*:-native}
for target in $targets; do
	tflags=
	strip=0
	if [ "$target" = native ]; then
		[ "$(uname -s)" = Darwin ] && strip=1
	else
		tflags="--target=$target"
		case $target in *apple*|*darwin*) strip=1 ;; esac
	fi
	tflags="$tflags $(engine_target_flags "$target")"
	$cc $tflags -c "$work/bad.c" -o "$work/bad-target.o"
	open "$work/target-open.txt" "$work/bad-target.o"
	for variant in "-O0 ." "-O1 -DNDEBUG" "-O2 ." "-O2 -DNDEBUG"; do
		opt=${variant% *}
		ndebug=${variant#* }
		[ "$ndebug" = . ] && ndebug=
		d=$work/obj
		rm -rf "$d"; mkdir -p "$d/libc"
		for f in quickjs libregexp libunicode cutils dtoa; do
			$cc $tflags $opt $ndebug $free -I"$libc" -I"$engine" -DCONFIG_VERSION=\"$version\" -c "$engine/$f.c" -o "$d/$f.o"
		done
		for f in "$libc"/string/*.c "$libc"/math/*.c "$libc"/math.c "$libc"/stdio.c "$libc"/abort.c; do
			o=$(basename "$f" .c)
			case $f in */string/*) o=string_$o ;; */math/*) o=math_$o ;; esac
			$cc $tflags $opt $ndebug $free -I"$libc" -I"$libc/internal" -I"$engine" -include "$libc/internal/features.h" -c "$f" -o "$d/libc/$o.o"
		done
		label="$target $opt $ndebug"
		open "$work/engine-open.txt" "$d"/*.o
		check "$label, engine files" "$work/engine-open.txt" "$work/all.txt"
		open "$work/both-open.txt" "$d"/*.o "$d"/libc/*.o
		check "$label, engine files with libc" "$work/both-open.txt" "$work/outside.txt"
		echo "check-symbols: $label: engine files use $(wc -l < "$work/engine-open.txt" | tr -d ' ') outside names, with libc $(wc -l < "$work/both-open.txt" | tr -d ' ')"
	done
done
[ $fail = 0 ] && echo "check-symbols: ok"
exit $fail
