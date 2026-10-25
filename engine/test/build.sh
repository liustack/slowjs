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
# gets run-test262 and api-test (see api-test.sh), and the object files: those of
# the engine here, those of its libc in libc/. CC picks another compiler. A
# sanitizer reports a finding and goes on, so that a run over test262 lists
# them all: run with ASAN_OPTIONS=halt_on_error=0 to keep AddressSanitizer
# going too.
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

# The flags of QuickJS's own Makefile: -fwrapv because the C relies on signed
# overflow wrapping, -funsigned-char for clang. The version is the one in the
# name of the pinned commit.
version=2026-06-04
common="-O2 -g -Wall -Wno-array-bounds -fwrapv -funsigned-char $san"
# What the engine and its libc are built with (see engine/libc/README.md):
# - no system headers or libc, so nothing outside engine/libc can creep in
# - -fno-math-errno, so that sqrt is an instruction and Math functions never
#   set errno
# - no fused multiply-add and no hardware fma branch in musl's math, so that
#   every target gives the same results
# - 32-bit BigInt limbs, as the engine has them in Go
free="-ffreestanding -nostdinc -fno-math-errno -ffp-contract=off -U__FP_FAST_FMA -U__FP_FAST_FMAF -U__SIZEOF_INT128__ -fno-stack-protector"
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
for f in "$libc"/string/*.c "$libc"/math/*.c "$libc"/math.c "$libc"/stdio.c; do
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
	$cc $flags $free -I"$libc" -I"$libc/internal" -include "$libc/internal/features.h" -c "$f" -o "$out/libc/$o.o"
	objs="$objs $out/libc/$o.o"
done

# The host and the programs use the system's headers and libc.
hostflags="$common -D_GNU_SOURCE -DCONFIG_VERSION=\"$version\" -I$engine -I$here"
$cc $hostflags -c "$here/host.c" -o "$out/host.o"
objs="$objs $out/host.o"
for f in run-test262 api-test; do
	$cc $hostflags -c "$here/$f.c" -o "$out/$f.o"
	$cc $san $objs "$out/$f.o" -o "$out/$f" -lpthread
done
echo "$out/run-test262"
