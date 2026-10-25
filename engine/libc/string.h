/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_LIBC_STRING_H
#define SLOWJS_LIBC_STRING_H

#include <stddef.h>

void *memcpy(void *restrict dest, const void *restrict src, size_t n);
void *memmove(void *dest, const void *src, size_t n);
void *memset(void *dest, int c, size_t n);
int memcmp(const void *l, const void *r, size_t n);
int bcmp(const void *l, const void *r, size_t n);
void *memchr(const void *s, int c, size_t n);
size_t strlen(const char *s);
int strcmp(const char *l, const char *r);
char *strchr(const char *s, int c);
char *strrchr(const char *s, int c);
/* musl's own names for the two functions it builds the others on. */
char *__strchrnul(const char *s, int c);
void *__memrchr(const void *m, int c, size_t n);

#endif
