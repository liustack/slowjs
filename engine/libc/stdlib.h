/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_LIBC_STDLIB_H
#define SLOWJS_LIBC_STDLIB_H

#include <stddef.h>

/* There is no malloc here: the engine has its own allocator (heap.c, behind
   js_malloc), and the engine files must not call the system's. */

/* There is no process to end. abort asks the host, with host_call and
   HOST_CALL_ABORT in engine/host.h. A trap would be a signal, and in Go a
   signal kills the whole process. The Go host makes a panic of it and the VM
   can only be closed. */
void abort(void) __attribute__((noreturn));
#define abs(x) __builtin_abs(x)
#define alloca(n) __builtin_alloca(n)

#endif
