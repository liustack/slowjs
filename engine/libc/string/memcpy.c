/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/* memcpy for amd64 and arm64, written here in place of musl's (musl's aligns
   the destination a byte at a time and shifts words for the rest, which is
   slow for the strings and buffers the engine copies).

   Up to 128 bytes a copy is two loads and two stores of one width w that
   overlap in the middle: the first w bytes and the last w bytes. w is 4 for 4
   to 7 bytes, 8 for 8 to 16, 16 for 17 to 32, 32 for 33 to 64 and 64 for 65 to
   128 (1 to 3 bytes: the first, the middle and the last). Go's runtime does it
   (src/runtime/memmove_amd64.s), and so does LLVM's libc
   (libc/src/string/memory_utils/x86_64/inline_memcpy.h). There is no loop and
   no branch on alignment. A copy of 129 bytes or more writes the first 16 bytes,
   brings the destination to a multiple of 16 and loops 128 bytes at a time with
   unaligned loads and aligned stores, then writes the last 128 bytes. The
   destination aligned and the source not is what glibc picks for its loops
   (sysdeps/x86_64/multiarch/memmove-vec-unaligned-erms.S). The loop is 8 loads
   and 8 stores with one counter: 19 instructions for 128 bytes on amd64, where
   64 bytes at a time took 13 for 64. Almost all the instructions memcpy runs in
   a React render are in this loop, since the strings that are concatenated are
   a few KB long. A loop of 256 bytes takes all 16 SSE registers, gains 1% to 3%
   on copies of 8 KB and more and loses 10% to 50% on copies of 130 to 300 bytes,
   since its last copy is 256 bytes.

   What it does not use: REP MOVSB (glibc does not use it on AMD either below
   its non-temporal threshold, and an unaligned destination made it up to 20
   times slower on Zen 3 and 4, glibc bug 30994), and AVX (the C cannot ask the
   CPU what it has, and a build for the base amd64 has SSE2 only). Loads and
   stores are the 16 byte moves of the base instruction sets (SSE2, NEON).

   __builtin_memcpy_inline takes a constant size and is always expanded into
   loads and stores, never a call, so this function cannot call itself.
   Source and destination do not overlap (memmove is the one that may). */
#include <string.h>
#include <stdint.h>

#define CP(d, s, n) __builtin_memcpy_inline((d), (s), (n))

/* A 16 byte move that needs no alignment (movups, ldr q). The loop below uses
   it rather than CP(d, s, 128), which LLVM expands into pairs of one load and
   one store through a single register, and unrolls four times. */
typedef long long v16 __attribute__((vector_size(16), aligned(1), may_alias));

/* 128 bytes: eight loads, then eight stores */
#define CP128(d, s) do { \
	const v16 *s_ = (const v16 *)(s); \
	v16 *d_ = (v16 *)(d); \
	v16 a0 = s_[0], a1 = s_[1], a2 = s_[2], a3 = s_[3]; \
	v16 a4 = s_[4], a5 = s_[5], a6 = s_[6], a7 = s_[7]; \
	d_[0] = a0; d_[1] = a1; d_[2] = a2; d_[3] = a3; \
	d_[4] = a4; d_[5] = a5; d_[6] = a6; d_[7] = a7; \
} while (0)

void *memcpy(void *restrict dest, const void *restrict src, size_t n)
{
	unsigned char *d = dest;
	const unsigned char *s = src;

	if (n <= 16) {
		if (n >= 8) {
			uint64_t a, b;
			CP(&a, s, 8); CP(&b, s + n - 8, 8);
			CP(d, &a, 8); CP(d + n - 8, &b, 8);
		} else if (n >= 4) {
			uint32_t a, b;
			CP(&a, s, 4); CP(&b, s + n - 4, 4);
			CP(d, &a, 4); CP(d + n - 4, &b, 4);
		} else if (n) {
			/* 1 to 3 bytes: the first, the middle and the last */
			unsigned char a = s[0], b = s[n >> 1], c = s[n - 1];
			d[0] = a; d[n >> 1] = b; d[n - 1] = c;
		}
		return dest;
	}
	if (n <= 32) {
		CP(d, s, 16);
		CP(d + n - 16, s + n - 16, 16);
		return dest;
	}
	if (n <= 64) {
		CP(d, s, 32);
		CP(d + n - 32, s + n - 32, 32);
		return dest;
	}
	if (n <= 128) {
		CP(d, s, 64);
		CP(d + n - 64, s + n - 64, 64);
		return dest;
	}
	{
		unsigned char *dend = d + n;
		const unsigned char *send = s + n;
		size_t adv = 16 - ((uintptr_t)d & 15), i, lim;

		CP(d, s, 16);
		d += adv; s += adv; n -= adv;
		/* the loop leaves 1 to 128 bytes, which the last copy covers */
		lim = (n - 1) & ~(size_t)127;
		#pragma clang loop unroll(disable)
		for (i = 0; i < lim; i += 128)
			CP128(d + i, s + i);
		CP128(dend - 128, send - 128);
	}
	return dest;
}
