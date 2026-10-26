#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Build the native test host: the C tree of engine/ with its own small libc
# (engine/libc), and the files of this folder, which use the system's libc. The
# engine and its libc are built as the generated assembly will be: freestanding,
# with no system headers. It is a tool for testing the C and never part of a Go
# build.
#
#   engine/test/build.sh [MODE] [OUTDIR]
#
# MODE is plain (default), asan (AddressSanitizer), ubsan (UndefinedBehavior
# Sanitizer) or asan-ubsan (both). OUTDIR defaults to engine/test/out/MODE. It
# gets run-test262, jsrun (see compare.sh), mathdump and libmath (see math.sh),
# api-test (see api-test.sh), and the object files: those of the engine here,
# and those of its libc in libc/. CC picks another compiler. A sanitizer reports
# a finding and goes on, so that a run over test262 lists them all: run with
# ASAN_OPTIONS=halt_on_error=0 to keep AddressSanitizer going too.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
engine=$(dirname "$here")
mode=${1:-plain}
out=${2:-$here/out/$mode}
cc=${CC:-clang}

case $mode in
plain) san= ;;
asan) san='-fsanitize=address -fsanitize-recover=address -fno-omit-frame-pointer' ;;
ubsan) san='-fsanitize=undefined -fno-omit-frame-pointer' ;;
asan-ubsan) san='-fsanitize=address,undefined -fsanitize-recover=address -fno-omit-frame-pointer' ;;
*) echo "build.sh: unknown mode $mode (plain, asan, ubsan or asan-ubsan)" >&2; exit 2 ;;
esac

# The flags of the engine files and engine/libc are in flags.sh, which
# check-symbols.sh reads too.
. "$here/flags.sh"
common="-O2 -g -Wall -Wno-array-bounds $san"
free="$engine_flags $(engine_target_flags native)"
version=$engine_version
libc=$engine/libc

mkdir -p "$out/libc"
objs=
for f in quickjs libregexp libunicode cutils dtoa; do
	$cc $common $free -I"$libc" -I"$engine" -DCONFIG_VERSION=\"$version\" -c "$engine/$f.c" -o "$out/$f.o"
	objs="$objs $out/$f.o"
done

# The functions that read a word at a time (strlen and the like) read past the
# end of a string inside its last aligned word, which AddressSanitizer reports,
# so they are built without it.
for f in "$libc"/string/*.c "$libc"/math/*.c "$libc"/math.c "$libc"/stdio.c "$libc"/abort.c; do
	name=$(basename "$f" .c)
	flags=$common
	case $name in
	strlen|strchrnul|memchr) flags="$flags -fno-sanitize=address" ;;
	esac
	case $f in
	*/string/*) o=string_$name ;;
	*/math/*) o=math_$name ;;
	*) o=$name ;;
	esac
	$cc $flags $free -I"$libc" -I"$libc/internal" -I"$engine" -include "$libc/internal/features.h" -c "$f" -o "$out/libc/$o.o"
	objs="$objs $out/libc/$o.o"
done

# The host and the programs use the system's headers and libc.
hostflags="$common -fwrapv -funsigned-char -D_GNU_SOURCE -DCONFIG_VERSION=\"$version\" -I$engine -I$here"
$cc $hostflags -c "$here/host.c" -o "$out/host.o"
objs="$objs $out/host.o"
for f in run-test262 jsrun mathdump api-test; do
	$cc $hostflags -c "$here/$f.c" -o "$out/$f.o"
	$cc $san $objs "$out/$f.o" -o "$out/$f" -lpthread
done
# libmath calls the math functions of engine/libc, and nothing else of it. It is
# not given -ffreestanding and the system's libm, but -fno-builtin, so that
# every call it makes is a call of engine/libc.
$cc $hostflags -fno-builtin -c "$here/libmath.c" -o "$out/libmath.o"
$cc $san "$out/libmath.o" "$out"/libc/math_*.o "$out/libc/math.o" -o "$out/libmath"
echo "$out/run-test262"
