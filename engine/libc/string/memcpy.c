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
   brings the destination to a multiple of 16 and loops 64 bytes at a time with
   unaligned loads and aligned stores, then writes the last 64 bytes. The
   destination aligned and the source not is what glibc picks for its loops
   (sysdeps/x86_64/multiarch/memmove-vec-unaligned-erms.S).

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
		size_t adv;

		CP(d, s, 16);
		adv = 16 - ((uintptr_t)d & 15);
		d += adv; s += adv; n -= adv;
		while (n > 64) {
			CP(d, s, 64);
			d += 64; s += 64; n -= 64;
		}
		CP(dend - 64, send - 64, 64);
	}
	return dest;
}
