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
# MODE is one of
#   plain       the engine as it is, with its own allocator (default)
#   asan        AddressSanitizer, every block alone from the system malloc
#               (heap-system.c in place of heap.c and tlsf.c), so that red zones
#               and LeakSanitizer see each block
#   asan-heap   AddressSanitizer with the engine's own allocator, which poisons
#               what is free (free slots, free TLSF blocks, the rest of an arena)
#   ubsan       UndefinedBehaviorSanitizer
#   asan-ubsan  both, on the system malloc
#   fuzz        the libFuzzer target of the allocator (heap-fuzz), with
#               AddressSanitizer (poisoning) and UndefinedBehaviorSanitizer
#   tsan        ThreadSanitizer, for host-test alone (the host with stubs in
#               place of the engine): the engine itself runs in one thread
# OUTDIR defaults to engine/test/out/MODE. It gets run-test262, jsrun (see
# compare.sh), unit, render, mathdump and libmath (see math.sh), api-test (see api-test.sh), host-test (see
# host-test.sh), heap-test and, in asan-heap mode, poison-test (see heap.sh), and
# the object files: those of the engine here, and those of its
# libc in libc/ (heap-fuzz for fuzz). CC picks another compiler. A sanitizer
# reports a finding and goes on, so that a run over test262 lists them all: run
# with ASAN_OPTIONS=halt_on_error=0 to keep AddressSanitizer going too.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
engine=$(dirname "$here")
mode=${1:-plain}
out=${2:-$here/out/$mode}
cc=${CC:-clang}

# san: for every file. heapsan: for heap.c and tlsf.c, which read the memory
# they manage and must not be checked by AddressSanitizer (the poison is
# theirs to put and take). defs: the allocator mode, for every file. system: 1
# when every block comes from the system malloc.
heapsan=
defs=
system=0
fuzzflags=
case $mode in
plain) san= ;;
asan) san='-fsanitize=address -fsanitize-recover=address -fno-omit-frame-pointer'; defs=-DJS_HEAP_SYSTEM_MALLOC; system=1 ;;
asan-heap) san='-fsanitize=address -fsanitize-recover=address -fno-omit-frame-pointer'; heapsan='-fno-sanitize=address -fno-omit-frame-pointer'; defs=-DJS_HEAP_POISON ;;
ubsan) san='-fsanitize=undefined -fno-omit-frame-pointer' ;;
asan-ubsan) san='-fsanitize=address,undefined -fsanitize-recover=address -fno-omit-frame-pointer'; defs=-DJS_HEAP_SYSTEM_MALLOC; system=1 ;;
tsan) san='-fsanitize=thread -fno-omit-frame-pointer' ;;
fuzz) san='-fsanitize=address,undefined,fuzzer-no-link -fno-sanitize-recover=all -fno-omit-frame-pointer'; heapsan='-fsanitize=undefined,fuzzer-no-link -fno-sanitize-recover=all -fno-omit-frame-pointer'; defs=-DJS_HEAP_POISON; fuzzflags='-fsanitize=fuzzer' ;;
*) echo "build.sh: unknown mode $mode (plain, asan, asan-heap, ubsan, asan-ubsan, tsan or fuzz)" >&2; exit 2 ;;
esac
[ -n "$heapsan" ] || heapsan=$san

# The flags of the engine files and engine/libc are in flags.sh, which
# check-symbols.sh reads too.
. "$here/flags.sh"
common="-O2 -g -Wall -Wno-array-bounds $san"
free="$engine_flags $(engine_target_flags native)"
version=$engine_version
libc=$engine/libc

# The host and the programs use the system's headers and libc.
hostflags="$common -fwrapv -funsigned-char -D_GNU_SOURCE -DCONFIG_VERSION=\"$version\" -I$engine -I$here $defs"

engine_files="quickjs libregexp libunicode cutils dtoa"
[ $system = 1 ] || engine_files="$engine_files heap tlsf"

# build_engine DIR EXTRA-FLAGS: the engine files into DIR. The names of the
# objects are in $objs afterwards.
build_engine() {
	mkdir -p "$1"
	objs=
	for f in $engine_files; do
		s=$common
		case $f in heap|tlsf) s="-O2 -g -Wall -Wno-array-bounds $heapsan" ;; esac
		# shellcheck disable=SC2086
		$cc $s $free $defs $2 -I"$libc" -I"$engine" -DCONFIG_VERSION=\"$version\" -c "$engine/$f.c" -o "$1/$f.o"
		objs="$objs $1/$f.o"
	done
	if [ $system = 1 ]; then
		# every block from the system malloc: this file is the host's, not the engine's
		# shellcheck disable=SC2086
		$cc $hostflags $2 -c "$here/heap-system.c" -o "$1/heap-system.o"
		objs="$objs $1/heap-system.o"
	fi
}

# host-test links the host with stubs in place of the engine.
build_host_test() {
	mkdir -p "$out"
	$cc $hostflags -c "$here/host.c" -o "$out/host.o"
	$cc $hostflags -c "$here/host-test.c" -o "$out/host-test.o"
	$cc $san "$out/host.o" "$out/host-test.o" -o "$out/host-test" -lpthread
	echo "$out/host-test"
}
if [ "$mode" = tsan ]; then
	build_host_test
	exit 0
fi

mkdir -p "$out/libc"
# The functions that read a word at a time (strlen and the like) read past the
# end of a string inside its last aligned word, which AddressSanitizer reports,
# so they are built without it.
libcobjs=
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
	libcobjs="$libcobjs $out/libc/$o.o"
done

# heap-test tests the allocator with a fake host of its own and the checks of
# the heap on (JS_HEAP_CHECK): the engine files are built again with it. It
# needs the allocator of the engine, so the system malloc modes leave it out.
build_heap_test() { # PROGRAM EXTRA-FLAGS EXTRA-LINK-FLAGS
	mkdir -p "$out/check"
	build_engine "$out/check" -DJS_HEAP_CHECK
	# shellcheck disable=SC2086
	$cc $hostflags $2 -DJS_HEAP_CHECK -DHEAP_TEST_RUNTIME -c "$here/heap-test.c" -o "$out/check/heap-test.o"
	# shellcheck disable=SC2086
	$cc $san $3 $objs $libcobjs "$out/check/heap-test.o" -o "$out/$1" -lpthread
	echo "$out/$1"
}

if [ "$mode" = fuzz ]; then
	build_heap_test heap-fuzz -DHEAP_FUZZ "$fuzzflags"
	exit 0
fi

build_engine "$out" ""
engineobjs=$objs
build_host_test > /dev/null
# poison-test asks AddressSanitizer what the allocator poisoned
poisontest=
case $defs in *JS_HEAP_POISON*) poisontest=poison-test ;; esac
for f in run-test262 jsrun unit mathdump api-test render $poisontest; do
	[ -f "$here/$f.c" ] || continue
	$cc $hostflags -c "$here/$f.c" -o "$out/$f.o"
	$cc $san $engineobjs $libcobjs "$out/host.o" "$out/$f.o" -o "$out/$f" -lpthread
done
# libmath calls the math functions of engine/libc, and nothing else of it. It is
# not given -ffreestanding and the system's libm, but -fno-builtin, so that
# every call it makes is a call of engine/libc.
$cc $hostflags -fno-builtin -c "$here/libmath.c" -o "$out/libmath.o"
$cc $san "$out/libmath.o" "$out"/libc/math_*.o "$out/libc/math.o" -o "$out/libmath"
echo "$out/run-test262"
[ $system = 1 ] || build_heap_test heap-test "" ""
