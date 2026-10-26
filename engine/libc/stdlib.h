/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_LIBC_STDLIB_H
#define SLOWJS_LIBC_STDLIB_H

#include <stddef.h>

/* The allocator is not in this library: the engine brings its own, and the
   native test host still links the system's until it does. */
void *malloc(size_t size);
void *realloc(void *ptr, size_t size);
void free(void *ptr);
#if defined(__APPLE__)
size_t malloc_size(const void *ptr);
#else
size_t malloc_usable_size(void *ptr);
#endif

/* There is no process to end. abort asks the host, with host_call and
   HOST_CALL_ABORT in engine/host.h. A trap would be a signal, and in Go a
   signal kills the whole process. The Go host makes a panic of it and the VM
   can only be closed. */
void abort(void) __attribute__((noreturn));
#define abs(x) __builtin_abs(x)
#define alloca(n) __builtin_alloca(n)

#endif
