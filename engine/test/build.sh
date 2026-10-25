#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Build the native test host: the C tree of engine/ and the files of this
# folder, built by plain clang with the system libc and libm. It is a tool for
# testing the C and never part of a Go build.
#
#   engine/test/build.sh [MODE] [OUTDIR]
#
# MODE is plain (default), asan (AddressSanitizer), ubsan (UndefinedBehavior
# Sanitizer) or asan-ubsan (both). OUTDIR defaults to engine/test/out/MODE, and
# gets run-test262 and the object files. CC picks another compiler. A
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
cflags="-O2 -g -Wall -Wno-array-bounds -Wno-format-truncation -fwrapv -funsigned-char -D_GNU_SOURCE -DCONFIG_VERSION=\"$version\" -I$engine $san"

mkdir -p "$out"
objs=
for f in quickjs libregexp libunicode cutils dtoa; do
	# The engine reads the wall clock only through gettimeofday. host.c
	# answers with a fixed time, which also fixes the seed of Math.random.
	$cc $cflags -Dgettimeofday=host_gettimeofday -c "$engine/$f.c" -o "$out/$f.o"
	objs="$objs $out/$f.o"
done
for f in host run-test262; do
	$cc $cflags -I"$here" -c "$here/$f.c" -o "$out/$f.o"
	objs="$objs $out/$f.o"
done
$cc $san $objs -o "$out/run-test262" -lm -lpthread
echo "$out/run-test262"
