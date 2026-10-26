#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Test the string functions and snprintf of engine/libc against the system's,
# and vsnprintf against the table of formats the generator accepts.
#
#   engine/test/libc-test.sh [MODE]
#
# MODE is a mode of build.sh (default plain). The library is built with every
# function renamed to sl_NAME, so that it can sit in one program with the
# system's, from the same files and with the flags of build.sh. CC picks
# another compiler.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
engine=$(dirname "$here")
libc=$engine/libc
mode=${1:-plain}
cc=${CC:-clang}
out=$here/out/libc-test-$mode
. "$here/flags.sh"

case $mode in
plain) san= ;;
asan) san='-fsanitize=address -fno-omit-frame-pointer' ;;
ubsan) san='-fsanitize=undefined -fno-omit-frame-pointer' ;;
asan-ubsan) san='-fsanitize=address,undefined -fno-omit-frame-pointer' ;;
*) echo "libc-test.sh: unknown mode $mode" >&2; exit 2 ;;
esac

rename=
for name in memcpy memmove memset memcmp bcmp memchr strlen strcmp strchr strrchr \
	__strchrnul strchrnul __memrchr memrchr snprintf vsnprintf; do
	rename="$rename -D$name=sl_$name"
done

mkdir -p "$out"
objs=
for f in "$libc"/string/*.c "$libc"/stdio.c; do
	name=$(basename "$f" .c)
	flags="-O2 -g -Wall $san"
	case $name in
	strlen|strchrnul|memchr) flags="$flags -fno-sanitize=address" ;;
	esac
	# stdio.c is built with a stdarg.h that counts the arguments it takes
	count=
	[ "$name" = stdio ] && count="-I$here/counting"
	$cc $flags $engine_flags $(engine_target_flags native) $rename $count \
		-I"$libc" -I"$libc/internal" -include "$libc/internal/features.h" -c "$f" -o "$out/$name.o"
	objs="$objs $out/$name.o"
done
$cc -O2 -g -Wall -Wno-format -Wno-format-security $san -c "$here/libc-test.c" -o "$out/libc-test.o"
$cc $san $objs "$out/libc-test.o" -o "$out/libc-test"
"$out/libc-test" "$(dirname "$engine")/internal/goasm/gen/testdata/formats/cases.txt"
