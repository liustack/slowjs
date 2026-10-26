# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# The compile flags of the engine files and engine/libc, in one place. build.sh
# and check-symbols.sh read them, so that what is built and what is checked
# cannot differ. Source this file.
#
# - no system headers or libc, so nothing outside engine/libc can creep in.
#   -ffreestanding also means -fno-builtin: a call of pow stays a call
# - -fno-math-errno, so that sqrt is an instruction and Math functions never
#   set errno
# - no fused multiply-add and no hardware fma branch in musl's math, so that
#   every target gives the same results
# - 32-bit BigInt limbs, as the engine has them in Go
# - -fwrapv and -funsigned-char, the flags of QuickJS's own Makefile for clang:
#   the C relies on signed overflow wrapping
engine_version=2026-06-04
engine_flags="-ffreestanding -nostdinc -fno-math-errno -ffp-contract=off -U__FP_FAST_FMA -U__FP_FAST_FMAF -U__SIZEOF_INT128__ -fno-stack-protector -fwrapv -funsigned-char"

# The flags a target needs on top of those: engine_target_flags native, or a
# clang target triple. On arm64 the compiler must not turn an atomic into a
# call of the runtime's outline atomics (__aarch64_*), a symbol nobody defines.
engine_target_flags() {
	t=$1
	[ "$t" = native ] && t=$(uname -m)
	case $t in
	aarch64* | arm64*) echo "-mno-outline-atomics" ;;
	esac
}
