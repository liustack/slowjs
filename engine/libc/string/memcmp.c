/*
 * Copyright (c) 2026 The liustack/beastjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE at the
 * root of this repository.
 */
/* memcmp for amd64 and arm64, written here in place of musl's byte loop.
   While 8 bytes or more are left it compares 8 at a time. The first word that
   differs gives the first differing byte (the lowest set bit of the xor, as
   both targets are little endian), and the result is what the byte loop gives:
   the difference of those two bytes as unsigned char. The rest, under 8 bytes,
   goes through the byte loop. The words are read with __builtin_memcpy, so no
   load is unaligned in the C sense, and no byte past the end is read. */
#include <string.h>

#if !defined(__BYTE_ORDER__) || __BYTE_ORDER__ != __ORDER_LITTLE_ENDIAN__
#error "memcmp.c needs a little endian target"
#endif

int memcmp(const void *vl, const void *vr, size_t n)
{
	const unsigned char *l=vl, *r=vr;
	for (; n >= 8; n -= 8, l += 8, r += 8) {
		unsigned long long a, b;
		__builtin_memcpy(&a, l, 8);
		__builtin_memcpy(&b, r, 8);
		if (a != b) {
			int s = __builtin_ctzll(a ^ b) & ~7;
			return (int)((a >> s) & 0xff) - (int)((b >> s) & 0xff);
		}
	}
	for (; n && *l == *r; n--, l++, r++);
	return n ? *l-*r : 0;
}
