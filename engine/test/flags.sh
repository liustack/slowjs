# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# The compile flags of the engine files and engine/libc, in one place. build.sh,
# check-symbols.sh, cycles.sh and libc-test.sh read them, and so does the
# generator (internal/goasm/gen, which runs this file in sh and adds only
# flags of its own), so that what is built and what is checked cannot differ.
# Source this file, and keep it to the two assignments and the function below:
# internal/goasm/gen/flags_test.go reads engine_flags by its text.
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
# - -Werror=format, so that a format and its arguments agree, and
#   -Werror=format-nonliteral, so that every format is a string constant, or the
#   format of a function that has the printf attribute and passes it on. The
#   small libc does only the formats the engine uses and aborts on the rest at
#   run time, and the generator checks the constants (internal/goasm/gen/formats.go)
#   but cannot see a format that is not one
engine_version=2026-06-04
engine_flags="-ffreestanding -nostdinc -fno-math-errno -ffp-contract=off -U__FP_FAST_FMA -U__FP_FAST_FMAF -U__SIZEOF_INT128__ -fno-stack-protector -fwrapv -funsigned-char -Werror=format -Werror=format-nonliteral"

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
